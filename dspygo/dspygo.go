// Package dspygo provides Go bindings for [DSPy], the framework for
// programming — rather than prompting — language models.
//
// The Python package is auto-installed via uv on first use.
//
//	client, _ := dspygo.New(dspygo.WithLM("openai/gpt-4o-mini"))
//	defer client.Close()
//
//	cot := client.ChainOfThought("question -> answer")
//	result, _ := cot.Call(dspygo.KV{"question": "What is 2+2?"})
//	fmt.Println(result.Get("answer"))
//
// [DSPy]: https://dspy.ai
package dspygo

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/i2y/pyffi"
)

// KV is a map of input key-value pairs for module calls.
type KV = map[string]any

// Client wraps a configured DSPy runtime.
type Client struct {
	rt *pyffi.Runtime
}

// Option configures a Client.
type Option func(*clientConfig)

type clientConfig struct {
	lm     string
	apiKey string
}

// WithLM sets the language model (e.g. "openai/gpt-4o-mini", "anthropic/claude-sonnet-4-5-20250514").
func WithLM(model string) Option {
	return func(c *clientConfig) { c.lm = model }
}

// WithAPIKey sets the API key. If not set, uses the provider's environment variable.
func WithAPIKey(key string) Option {
	return func(c *clientConfig) { c.apiKey = key }
}

// New creates a new DSPy client.
func New(opts ...Option) (*Client, error) {
	var cfg clientConfig
	for _, o := range opts {
		o(&cfg)
	}

	rt, err := pyffi.New(pyffi.Dependencies("dspy"))
	if err != nil {
		return nil, fmt.Errorf("dspygo: %w", err)
	}

	// Configure DSPy
	var lmParts []string
	if cfg.lm != "" {
		lmParts = append(lmParts, fmt.Sprintf("%q", cfg.lm))
	} else {
		lmParts = append(lmParts, `"openai/gpt-4o-mini"`)
	}
	if cfg.apiKey != "" {
		lmParts = append(lmParts, fmt.Sprintf("api_key=%q", cfg.apiKey))
	}

	code := fmt.Sprintf(`
import dspy
_dspy_lm = dspy.LM(%s)
dspy.configure(lm=_dspy_lm)
`, strings.Join(lmParts, ", "))

	if err := rt.Exec(code); err != nil {
		rt.Close()
		return nil, fmt.Errorf("dspygo: configure: %w", err)
	}

	return &Client{rt: rt}, nil
}

// Close releases all resources.
func (c *Client) Close() error {
	return c.rt.Close()
}

// Runtime returns the underlying pyffi Runtime for advanced use.
func (c *Client) Runtime() *pyffi.Runtime {
	return c.rt
}

// Predict creates a dspy.Predict module from a signature string.
//
//	predict := client.Predict("question -> answer")
func (c *Client) Predict(signature string) *Module {
	return &Module{rt: c.rt, kind: "Predict", signature: signature}
}

// ChainOfThought creates a dspy.ChainOfThought module from a signature string.
// This includes reasoning steps before the output.
//
//	cot := client.ChainOfThought("question -> answer: float")
func (c *Client) ChainOfThought(signature string) *Module {
	return &Module{rt: c.rt, kind: "ChainOfThought", signature: signature}
}

// Module wraps a DSPy module (Predict, ChainOfThought, etc.).
type Module struct {
	rt        *pyffi.Runtime
	kind      string // "Predict", "ChainOfThought"
	signature string
	typedSig  *Signature
	optimized bool
}

// Call invokes the module with the given inputs and returns the prediction.
func (m *Module) Call(inputs KV) (*Prediction, error) {
	// Build kwargs string
	kwStr := buildKwargsString(inputs)

	var code strings.Builder

	if m.optimized {
		// Use the already-compiled optimized module
		code.WriteString(fmt.Sprintf("_dspy_pred = _dspy_optimized(%s)\n", kwStr))
	} else if m.typedSig != nil {
		// Build typed signature class
		sigClass := "_DspyCallSig"
		code.WriteString("from typing import Literal\n")
		code.WriteString(buildSignatureCode(*m.typedSig, sigClass))
		code.WriteString(fmt.Sprintf("_dspy_mod = dspy.%s(%s)\n", m.kind, sigClass))
		code.WriteString(fmt.Sprintf("_dspy_pred = _dspy_mod(%s)\n", kwStr))
	} else {
		code.WriteString(fmt.Sprintf("_dspy_mod = dspy.%s(%q)\n", m.kind, m.signature))
		code.WriteString(fmt.Sprintf("_dspy_pred = _dspy_mod(%s)\n", kwStr))
	}

	if err := m.rt.Exec(code.String()); err != nil {
		return nil, fmt.Errorf("dspygo: %s: %w", m.kind, err)
	}

	mainMod, _ := m.rt.Import("__main__")
	defer mainMod.Close()

	predObj := mainMod.Attr("_dspy_pred")
	if predObj == nil {
		return nil, fmt.Errorf("dspygo: no prediction result")
	}

	return extractPrediction(predObj), nil
}

func buildKwargsString(inputs KV) string {
	var parts []string
	for k, v := range inputs {
		switch val := v.(type) {
		case string:
			parts = append(parts, fmt.Sprintf("%s=%q", k, val))
		case []string:
			b, _ := json.Marshal(val)
			parts = append(parts, fmt.Sprintf("%s=%s", k, string(b)))
		default:
			parts = append(parts, fmt.Sprintf("%s=%v", k, val))
		}
	}
	return strings.Join(parts, ", ")
}

// Prediction holds the output of a DSPy module call.
// All fields are eagerly extracted; Prediction does not require Close().
type Prediction struct {
	fields map[string]string
}

// Get returns the value of a field by name.
func (p *Prediction) Get(key string) string {
	return p.fields[key]
}

// Fields returns all output fields as a map.
func (p *Prediction) Fields() map[string]string {
	return p.fields
}

// --- Typed Signatures ---

// Field defines an input or output field for a typed Signature.
type Field struct {
	Name        string
	Description string // optional description for the field
	Type        string // Python type hint: "str", "float", "int", "bool", "list[str]", or Literal e.g. `Literal["positive","negative"]`
}

// Signature defines a typed input/output contract for a DSPy module.
type Signature struct {
	Doc     string  // docstring (used as the task description)
	Inputs  []Field
	Outputs []Field
}

// buildSignatureCode generates Python code to define a dspy.Signature subclass.
func buildSignatureCode(sig Signature, className string) string {
	var code strings.Builder
	code.WriteString("from typing import Literal\n")
	code.WriteString(fmt.Sprintf("class %s(dspy.Signature):\n", className))
	if sig.Doc != "" {
		code.WriteString(fmt.Sprintf("    %q\n", sig.Doc))
	} else {
		code.WriteString("    pass\n")
	}
	for _, f := range sig.Inputs {
		typ := f.Type
		if typ == "" {
			typ = "str"
		}
		if f.Description != "" {
			code.WriteString(fmt.Sprintf("    %s: %s = dspy.InputField(desc=%q)\n", f.Name, typ, f.Description))
		} else {
			code.WriteString(fmt.Sprintf("    %s: %s = dspy.InputField()\n", f.Name, typ))
		}
	}
	for _, f := range sig.Outputs {
		typ := f.Type
		if typ == "" {
			typ = "str"
		}
		if f.Description != "" {
			code.WriteString(fmt.Sprintf("    %s: %s = dspy.OutputField(desc=%q)\n", f.Name, typ, f.Description))
		} else {
			code.WriteString(fmt.Sprintf("    %s: %s = dspy.OutputField()\n", f.Name, typ))
		}
	}
	return code.String()
}

// PredictSig creates a dspy.Predict module from a typed Signature.
func (c *Client) PredictSig(sig Signature) *Module {
	return &Module{rt: c.rt, kind: "Predict", typedSig: &sig}
}

// ChainOfThoughtSig creates a dspy.ChainOfThought module from a typed Signature.
func (c *Client) ChainOfThoughtSig(sig Signature) *Module {
	return &Module{rt: c.rt, kind: "ChainOfThought", typedSig: &sig}
}

// --- Module Pipelines ---

// Pipeline defines a multi-step DSPy module as a Python class.
// Steps are executed in order; each step's outputs are available to subsequent steps.
type Pipeline struct {
	rt    *pyffi.Runtime
	steps []PipelineStep
}

// PipelineStep defines one step in a pipeline.
type PipelineStep struct {
	Name      string // variable name for this step's module
	Kind      string // "Predict" or "ChainOfThought"
	Signature string // signature string
}

// NewPipeline creates a multi-step pipeline.
func (c *Client) NewPipeline(steps ...PipelineStep) *Pipeline {
	return &Pipeline{rt: c.rt, steps: steps}
}

// Run executes the pipeline. The inputs are passed to the first step.
// Each step's outputs become available as inputs to subsequent steps.
// Returns the final step's prediction.
func (p *Pipeline) Run(inputs KV) (*Prediction, error) {
	var code strings.Builder
	code.WriteString("import dspy\n")

	// Define modules
	for _, step := range p.steps {
		code.WriteString(fmt.Sprintf("_dspy_step_%s = dspy.%s(%q)\n", step.Name, step.Kind, step.Signature))
	}

	// Build execution: chain steps, passing all accumulated KV
	// First step uses the user inputs
	var inputParts []string
	for k, v := range inputs {
		switch val := v.(type) {
		case string:
			inputParts = append(inputParts, fmt.Sprintf("%s=%q", k, val))
		case []string:
			b, _ := json.Marshal(val)
			inputParts = append(inputParts, fmt.Sprintf("%s=%s", k, string(b)))
		default:
			inputParts = append(inputParts, fmt.Sprintf("%s=%v", k, val))
		}
	}

	code.WriteString(fmt.Sprintf("_dspy_pipe_result_%s = _dspy_step_%s(%s)\n",
		p.steps[0].Name, p.steps[0].Name, strings.Join(inputParts, ", ")))

	// Subsequent steps: pass previous result fields as kwargs
	for i := 1; i < len(p.steps); i++ {
		prev := p.steps[i-1].Name
		cur := p.steps[i].Name
		code.WriteString(fmt.Sprintf("_dspy_pipe_result_%s = _dspy_step_%s(**_dspy_pipe_result_%s.toDict())\n",
			cur, cur, prev))
	}

	// Final result
	lastStep := p.steps[len(p.steps)-1].Name
	code.WriteString(fmt.Sprintf("_dspy_pipe_final = _dspy_pipe_result_%s\n", lastStep))

	if err := p.rt.Exec(code.String()); err != nil {
		return nil, fmt.Errorf("dspygo: pipeline: %w", err)
	}

	mainMod, _ := p.rt.Import("__main__")
	defer mainMod.Close()

	predObj := mainMod.Attr("_dspy_pipe_final")
	if predObj == nil {
		return nil, fmt.Errorf("dspygo: pipeline: no result")
	}

	return extractPrediction(predObj), nil
}

// --- Optimizer ---

// Example is a training example for DSPy optimizers.
type Example struct {
	Inputs  KV
	Outputs KV // expected outputs (for metric evaluation)
}

// OptimizeConfig configures an optimization run.
type OptimizeConfig struct {
	Optimizer  string // "BootstrapRS" (default), "MIPROv2"
	Metric     string // Python expression for metric, e.g. "dspy.evaluate.answer_exact_match"
	NumThreads int    // parallelism for optimization
	MaxRounds  int    // max bootstrapping rounds (BootstrapRS)
	Auto       string // MIPROv2 auto setting: "light", "medium", "heavy"
}

// Optimize compiles a module with training examples to produce an optimized version.
// The optimized module has better prompts/few-shot examples selected by the optimizer.
func (c *Client) Optimize(mod *Module, trainset []Example, cfg OptimizeConfig) (*Module, error) {
	var code strings.Builder
	code.WriteString("import dspy\n")

	// Build trainset
	code.WriteString("_dspy_trainset = []\n")
	for _, ex := range trainset {
		var kvParts []string
		for k, v := range ex.Inputs {
			switch val := v.(type) {
			case string:
				kvParts = append(kvParts, fmt.Sprintf("%s=%q", k, val))
			default:
				kvParts = append(kvParts, fmt.Sprintf("%s=%v", k, val))
			}
		}
		for k, v := range ex.Outputs {
			switch val := v.(type) {
			case string:
				kvParts = append(kvParts, fmt.Sprintf("%s=%q", k, val))
			default:
				kvParts = append(kvParts, fmt.Sprintf("%s=%v", k, val))
			}
		}

		// Collect input field names for with_inputs()
		var inputNames []string
		for k := range ex.Inputs {
			inputNames = append(inputNames, fmt.Sprintf("%q", k))
		}
		code.WriteString(fmt.Sprintf("_dspy_trainset.append(dspy.Example(%s).with_inputs(%s))\n",
			strings.Join(kvParts, ", "), strings.Join(inputNames, ", ")))
	}

	// Build module
	sig := mod.signature
	if mod.typedSig != nil {
		sigClass := "_DspyOptSig"
		code.WriteString(buildSignatureCode(*mod.typedSig, sigClass))
		sig = sigClass
	}
	code.WriteString(fmt.Sprintf("_dspy_opt_mod = dspy.%s(%q)\n", mod.kind, sig))

	// Build optimizer
	optimizer := cfg.Optimizer
	if optimizer == "" {
		optimizer = "BootstrapRS"
	}
	metric := cfg.Metric
	if metric == "" {
		metric = "dspy.evaluate.answer_exact_match"
	}

	switch optimizer {
	case "BootstrapRS":
		numThreads := cfg.NumThreads
		if numThreads <= 0 {
			numThreads = 1
		}
		maxRounds := cfg.MaxRounds
		if maxRounds <= 0 {
			maxRounds = 1
		}
		code.WriteString(fmt.Sprintf(`
_dspy_optimizer = dspy.BootstrapRS(metric=%s, max_rounds=%d, num_threads=%d)
_dspy_optimized = _dspy_optimizer.compile(_dspy_opt_mod, trainset=_dspy_trainset)
`, metric, maxRounds, numThreads))
	case "MIPROv2":
		numThreads := cfg.NumThreads
		if numThreads <= 0 {
			numThreads = 1
		}
		auto := cfg.Auto
		if auto == "" {
			auto = "light"
		}
		code.WriteString(fmt.Sprintf(`
_dspy_optimizer = dspy.MIPROv2(metric=%s, auto=%q, num_threads=%d)
_dspy_optimized = _dspy_optimizer.compile(_dspy_opt_mod, trainset=_dspy_trainset)
`, metric, auto, numThreads))
	default:
		return nil, fmt.Errorf("dspygo: unknown optimizer %q", optimizer)
	}

	if err := c.rt.Exec(code.String()); err != nil {
		return nil, fmt.Errorf("dspygo: optimize: %w", err)
	}

	// The optimized module lives in __main__._dspy_optimized.
	// We wrap it as a Module that calls it directly.
	return &Module{
		rt:        c.rt,
		kind:      mod.kind,
		signature: sig,
		optimized: true,
	}, nil
}

func extractPrediction(obj *pyffi.Object) *Prediction {
	defer obj.Close()
	pred := &Prediction{fields: make(map[string]string)}

	// Get all keys from the prediction via toDict()
	toDictFn := obj.Attr("toDict")
	if toDictFn != nil {
		dictObj, err := toDictFn.Call()
		toDictFn.Close()
		if err == nil && dictObj != nil {
			m, err := dictObj.GoMap()
			if err == nil {
				for k, v := range m {
					pred.fields[k] = fmt.Sprintf("%v", v)
				}
			}
			dictObj.Close()
		}
	}

	return pred
}
