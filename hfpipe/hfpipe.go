// Package hfpipe provides Go bindings for [Hugging Face Transformers] pipelines,
// enabling local ML inference from Go — text generation, classification,
// summarization, translation, embeddings, and more.
//
// The Python packages are auto-installed via uv on first use.
//
//	pipe, _ := hfpipe.New("text-generation", "openai-community/gpt2")
//	defer pipe.Close()
//
//	results, _ := pipe.Run("The meaning of life is")
//	fmt.Println(results[0]["generated_text"])
//
// [Hugging Face Transformers]: https://huggingface.co/docs/transformers
package hfpipe

import (
	"encoding/json"
	"fmt"

	"github.com/i2y/pyffi"
)

// Pipeline wraps a Hugging Face transformers pipeline.
type Pipeline struct {
	rt   *pyffi.Runtime
	pipe *pyffi.Object
}

// Option configures a Pipeline.
type Option func(*pipeConfig)

type pipeConfig struct {
	device    string // "cpu", "cuda", "mps", "auto"
	dtype     string // "float16", "bfloat16", ""
	batchSize int
}

// WithDevice sets the device for inference.
// Values: "cpu" (default), "cuda", "cuda:0", "mps", "auto".
func WithDevice(device string) Option {
	return func(c *pipeConfig) { c.device = device }
}

// WithDtype sets the model precision.
// Values: "float16", "bfloat16". Empty string uses default precision.
func WithDtype(dtype string) Option {
	return func(c *pipeConfig) { c.dtype = dtype }
}

// WithBatchSize sets the batch size for inference.
func WithBatchSize(n int) Option {
	return func(c *pipeConfig) { c.batchSize = n }
}

// New creates a new pipeline for the given task and model.
//
// Supported tasks include: "text-generation", "text-classification",
// "summarization", "translation", "fill-mask", "question-answering",
// "feature-extraction", "sentiment-analysis", "zero-shot-classification",
// "automatic-speech-recognition", "image-classification", and more.
func New(task, model string, opts ...Option) (*Pipeline, error) {
	var cfg pipeConfig
	for _, o := range opts {
		o(&cfg)
	}

	rt, err := pyffi.New(pyffi.Dependencies("transformers", "torch", "accelerate"))
	if err != nil {
		return nil, fmt.Errorf("hfpipe: %w", err)
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("task=%q", task))
	parts = append(parts, fmt.Sprintf("model=%q", model))

	if cfg.device != "" {
		if cfg.device == "auto" {
			parts = append(parts, `device_map="auto"`)
		} else {
			parts = append(parts, fmt.Sprintf("device=%q", cfg.device))
		}
	}
	if cfg.dtype == "float16" {
		parts = append(parts, "torch_dtype=torch.float16")
	} else if cfg.dtype == "bfloat16" {
		parts = append(parts, "torch_dtype=torch.bfloat16")
	}
	if cfg.batchSize > 0 {
		parts = append(parts, fmt.Sprintf("batch_size=%d", cfg.batchSize))
	}

	code := "import torch\nfrom transformers import pipeline as _hf_pipeline\n"
	code += fmt.Sprintf("_hfpipe = _hf_pipeline(%s)\n", joinParts(parts))

	if err := rt.Exec(code); err != nil {
		rt.Close()
		return nil, fmt.Errorf("hfpipe: load pipeline: %w", err)
	}

	mainMod, _ := rt.Import("__main__")
	defer mainMod.Close()
	obj := mainMod.Attr("_hfpipe")
	if obj == nil {
		rt.Close()
		return nil, fmt.Errorf("hfpipe: pipeline not found")
	}

	return &Pipeline{rt: rt, pipe: obj}, nil
}

// Close releases the pipeline and Runtime.
func (p *Pipeline) Close() error {
	p.pipe.Close()
	return p.rt.Close()
}

// Run executes the pipeline with one or more inputs.
// Returns a list of result maps, one per input.
func (p *Pipeline) Run(inputs ...string) ([]map[string]any, error) {
	if len(inputs) == 0 {
		return nil, nil
	}

	mainMod, _ := p.rt.Import("__main__")
	defer mainMod.Close()

	// Set inputs
	if len(inputs) == 1 {
		obj := p.rt.FromString(inputs[0])
		defer obj.Close()
		mainMod.SetAttr("_hfpipe_input", obj)
	} else {
		args := make([]any, len(inputs))
		for i, s := range inputs {
			args[i] = s
		}
		listObj, err := p.rt.NewList(args...)
		if err != nil {
			return nil, fmt.Errorf("hfpipe: create input list: %w", err)
		}
		defer listObj.Close()
		mainMod.SetAttr("_hfpipe_input", listObj)
	}

	code := `
import json as _json
_hfpipe_raw = _hfpipe(_hfpipe_input)
# Normalize: pipeline returns list of dicts or list of list of dicts
if _hfpipe_raw and isinstance(_hfpipe_raw[0], list):
    _hfpipe_result = _json.dumps([item for sublist in _hfpipe_raw for item in sublist], default=str)
else:
    _hfpipe_result = _json.dumps(_hfpipe_raw, default=str)
`
	if err := p.rt.Exec(code); err != nil {
		return nil, fmt.Errorf("hfpipe: run: %w", err)
	}

	resultObj := mainMod.Attr("_hfpipe_result")
	if resultObj == nil {
		return nil, fmt.Errorf("hfpipe: no result")
	}
	defer resultObj.Close()

	jsonStr, _ := resultObj.GoString()
	var results []map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &results); err != nil {
		return nil, fmt.Errorf("hfpipe: parse result: %w", err)
	}

	return results, nil
}

// RunKw executes the pipeline with inputs and extra keyword arguments.
// Use this for task-specific parameters like max_new_tokens, num_return_sequences, etc.
func (p *Pipeline) RunKw(input string, kwargs map[string]any) ([]map[string]any, error) {
	mainMod, _ := p.rt.Import("__main__")
	defer mainMod.Close()

	obj := p.rt.FromString(input)
	defer obj.Close()
	mainMod.SetAttr("_hfpipe_input", obj)

	kwargsJSON, _ := json.Marshal(kwargs)
	mainMod.SetAttr("_hfpipe_kwargs_json", p.rt.FromString(string(kwargsJSON)))

	code := `
import json as _json
_hfpipe_kwargs = _json.loads(_hfpipe_kwargs_json)
_hfpipe_raw = _hfpipe(_hfpipe_input, **_hfpipe_kwargs)
if _hfpipe_raw and isinstance(_hfpipe_raw[0], list):
    _hfpipe_result = _json.dumps([item for sublist in _hfpipe_raw for item in sublist], default=str)
else:
    _hfpipe_result = _json.dumps(_hfpipe_raw, default=str)
`
	if err := p.rt.Exec(code); err != nil {
		return nil, fmt.Errorf("hfpipe: run: %w", err)
	}

	resultObj := mainMod.Attr("_hfpipe_result")
	if resultObj == nil {
		return nil, fmt.Errorf("hfpipe: no result")
	}
	defer resultObj.Close()

	jsonStr, _ := resultObj.GoString()
	var results []map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &results); err != nil {
		return nil, fmt.Errorf("hfpipe: parse result: %w", err)
	}

	return results, nil
}

func joinParts(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += ", "
		}
		result += p
	}
	return result
}
