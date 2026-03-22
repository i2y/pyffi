// Package peftgo provides Go bindings for [PEFT] (Parameter-Efficient Fine-Tuning),
// enabling LoRA and other adapter-based fine-tuning of large models from Go.
//
//	model, _ := peftgo.NewModel("meta-llama/Llama-3-8B", peftgo.WithDevice("cuda"))
//	defer model.Close()
//
//	model.ApplyLoRA(peftgo.LoRAConfig{Rank: 16, Alpha: 32})
//	model.Train(dataset, peftgo.TrainConfig{Epochs: 3})
//	model.SaveAdapter("./my-adapter")
//
// [PEFT]: https://github.com/huggingface/peft
package peftgo

import (
	"fmt"
	"strings"

	"github.com/i2y/pyffi"
)

// Model wraps a base model with PEFT adapter support.
type Model struct {
	rt    *pyffi.Runtime
	model *pyffi.Object
	peft  bool // true if PEFT adapter is applied
}

// ModelOption configures model loading.
type ModelOption func(*modelConfig)

type modelConfig struct {
	device string
	dtype  string
}

// WithDevice sets the device ("cuda", "mps", "cpu", "auto").
func WithDevice(device string) ModelOption {
	return func(c *modelConfig) { c.device = device }
}

// WithDtype sets the precision ("float16", "bfloat16", "float32").
func WithDtype(dtype string) ModelOption {
	return func(c *modelConfig) { c.dtype = dtype }
}

// NewModel loads a pretrained model from the Hugging Face Hub.
func NewModel(modelID string, opts ...ModelOption) (*Model, error) {
	var cfg modelConfig
	for _, o := range opts {
		o(&cfg)
	}

	rt, err := pyffi.New(pyffi.Dependencies("peft", "transformers", "torch", "accelerate"))
	if err != nil {
		return nil, fmt.Errorf("peftgo: %w", err)
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("%q", modelID))
	if cfg.device == "auto" {
		parts = append(parts, `device_map="auto"`)
	}
	if cfg.dtype == "float16" {
		parts = append(parts, "torch_dtype=torch.float16")
	} else if cfg.dtype == "bfloat16" {
		parts = append(parts, "torch_dtype=torch.bfloat16")
	}

	code := fmt.Sprintf(`
import torch
from transformers import AutoModelForCausalLM, AutoTokenizer
_peft_model = AutoModelForCausalLM.from_pretrained(%s)
_peft_tokenizer = AutoTokenizer.from_pretrained(%q)
`, strings.Join(parts, ", "), modelID)

	if cfg.device != "" && cfg.device != "auto" {
		code += fmt.Sprintf("_peft_model = _peft_model.to(%q)\n", cfg.device)
	}

	if err := rt.Exec(code); err != nil {
		rt.Close()
		return nil, fmt.Errorf("peftgo: load model: %w", err)
	}

	mainMod, _ := rt.Import("__main__")
	defer mainMod.Close()
	model := mainMod.Attr("_peft_model")
	if model == nil {
		rt.Close()
		return nil, fmt.Errorf("peftgo: model not found")
	}

	return &Model{rt: rt, model: model}, nil
}

// Close releases all resources.
func (m *Model) Close() error {
	m.model.Close()
	return m.rt.Close()
}

// Runtime returns the underlying pyffi Runtime.
func (m *Model) Runtime() *pyffi.Runtime { return m.rt }

// LoRAConfig configures LoRA adapter parameters.
type LoRAConfig struct {
	Rank          int      // LoRA rank (r). Default: 8
	Alpha         int      // LoRA alpha scaling. Default: 16
	Dropout       float64  // LoRA dropout. Default: 0.0
	TargetModules []string // Modules to apply LoRA to. Empty = auto-detect
	TaskType      string   // "CAUSAL_LM" (default), "SEQ_2_SEQ_LM", "TOKEN_CLS", "SEQ_CLS"
}

// ApplyLoRA wraps the model with a LoRA adapter.
func (m *Model) ApplyLoRA(cfg LoRAConfig) error {
	rank := cfg.Rank
	if rank <= 0 {
		rank = 8
	}
	alpha := cfg.Alpha
	if alpha <= 0 {
		alpha = 16
	}
	taskType := cfg.TaskType
	if taskType == "" {
		taskType = "CAUSAL_LM"
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("r=%d", rank))
	parts = append(parts, fmt.Sprintf("lora_alpha=%d", alpha))
	parts = append(parts, fmt.Sprintf("task_type=peft.TaskType.%s", taskType))
	if cfg.Dropout > 0 {
		parts = append(parts, fmt.Sprintf("lora_dropout=%f", cfg.Dropout))
	}
	if len(cfg.TargetModules) > 0 {
		mods := "["
		for i, mod := range cfg.TargetModules {
			if i > 0 {
				mods += ", "
			}
			mods += fmt.Sprintf("%q", mod)
		}
		mods += "]"
		parts = append(parts, fmt.Sprintf("target_modules=%s", mods))
	}

	code := fmt.Sprintf(`
import peft
_peft_config = peft.LoraConfig(%s)
_peft_model = peft.get_peft_model(_peft_model, _peft_config)
`, strings.Join(parts, ", "))

	mainMod, _ := m.rt.Import("__main__")
	defer mainMod.Close()
	mainMod.SetAttr("_peft_model", m.model)

	if err := m.rt.Exec(code); err != nil {
		return fmt.Errorf("peftgo: apply lora: %w", err)
	}

	newModel := mainMod.Attr("_peft_model")
	if newModel == nil {
		return fmt.Errorf("peftgo: peft model not found")
	}
	m.model.Close()
	m.model = newModel
	m.peft = true

	return nil
}

// TrainableParams returns (trainable, total, percentage).
func (m *Model) TrainableParams() (int64, int64, float64) {
	mainMod, _ := m.rt.Import("__main__")
	defer mainMod.Close()
	mainMod.SetAttr("_peft_model", m.model)

	code := `
_tp = sum(p.numel() for p in _peft_model.parameters() if p.requires_grad)
_ap = sum(p.numel() for p in _peft_model.parameters())
_peft_trainable = _tp
_peft_total = _ap
_peft_pct = 100 * _tp / _ap if _ap > 0 else 0
`
	if err := m.rt.Exec(code); err != nil {
		return 0, 0, 0
	}

	trainObj := mainMod.Attr("_peft_trainable")
	totalObj := mainMod.Attr("_peft_total")
	pctObj := mainMod.Attr("_peft_pct")

	var trainable, total int64
	var pct float64
	if trainObj != nil {
		trainable, _ = trainObj.Int64()
		trainObj.Close()
	}
	if totalObj != nil {
		total, _ = totalObj.Int64()
		totalObj.Close()
	}
	if pctObj != nil {
		pct, _ = pctObj.Float64()
		pctObj.Close()
	}
	return trainable, total, pct
}

// SaveAdapter saves the PEFT adapter weights to a directory.
func (m *Model) SaveAdapter(path string) error {
	fn := m.model.Attr("save_pretrained")
	if fn == nil {
		return fmt.Errorf("peftgo: save_pretrained not found")
	}
	defer fn.Close()
	result, err := fn.Call(path)
	if err != nil {
		return fmt.Errorf("peftgo: save: %w", err)
	}
	if result != nil {
		result.Close()
	}
	return nil
}

// LoadAdapter loads a saved PEFT adapter onto a base model.
func LoadAdapter(baseModelID, adapterPath string, opts ...ModelOption) (*Model, error) {
	var cfg modelConfig
	for _, o := range opts {
		o(&cfg)
	}

	rt, err := pyffi.New(pyffi.Dependencies("peft", "transformers", "torch", "accelerate"))
	if err != nil {
		return nil, fmt.Errorf("peftgo: %w", err)
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("%q", baseModelID))
	if cfg.device == "auto" {
		parts = append(parts, `device_map="auto"`)
	}
	if cfg.dtype == "float16" {
		parts = append(parts, "torch_dtype=torch.float16")
	} else if cfg.dtype == "bfloat16" {
		parts = append(parts, "torch_dtype=torch.bfloat16")
	}

	code := fmt.Sprintf(`
import torch
from transformers import AutoModelForCausalLM, AutoTokenizer
from peft import PeftModel
_peft_base = AutoModelForCausalLM.from_pretrained(%s)
_peft_model = PeftModel.from_pretrained(_peft_base, %q)
_peft_tokenizer = AutoTokenizer.from_pretrained(%q)
`, strings.Join(parts, ", "), adapterPath, baseModelID)

	if cfg.device != "" && cfg.device != "auto" {
		code += fmt.Sprintf("_peft_model = _peft_model.to(%q)\n", cfg.device)
	}

	if err := rt.Exec(code); err != nil {
		rt.Close()
		return nil, fmt.Errorf("peftgo: load adapter: %w", err)
	}

	mainMod, _ := rt.Import("__main__")
	defer mainMod.Close()
	model := mainMod.Attr("_peft_model")
	if model == nil {
		rt.Close()
		return nil, fmt.Errorf("peftgo: model not found")
	}

	return &Model{rt: rt, model: model, peft: true}, nil
}

// Generate generates text using the model.
func (m *Model) Generate(prompt string, maxNewTokens int) (string, error) {
	mainMod, _ := m.rt.Import("__main__")
	defer mainMod.Close()
	mainMod.SetAttr("_peft_model", m.model)

	promptObj := m.rt.FromString(prompt)
	defer promptObj.Close()
	mainMod.SetAttr("_peft_prompt", promptObj)

	code := fmt.Sprintf(`
_peft_inputs = _peft_tokenizer(_peft_prompt, return_tensors="pt").to(_peft_model.device)
_peft_outputs = _peft_model.generate(**_peft_inputs, max_new_tokens=%d)
_peft_generated = _peft_tokenizer.decode(_peft_outputs[0], skip_special_tokens=True)
`, maxNewTokens)

	if err := m.rt.Exec(code); err != nil {
		return "", fmt.Errorf("peftgo: generate: %w", err)
	}

	result := mainMod.Attr("_peft_generated")
	if result == nil {
		return "", fmt.Errorf("peftgo: no generated text")
	}
	defer result.Close()
	return result.GoString()
}
