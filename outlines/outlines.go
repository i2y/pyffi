// Package outlines provides Go bindings for [Outlines], a library for
// structured generation from LLMs via constrained decoding.
//
// Outlines guarantees that LLM output matches the specified type — Literal
// choices, JSON schemas, regex patterns, or Pydantic models — during token
// generation, not as post-processing.
//
//	model, _ := outlines.NewOpenAI("gpt-4o")
//	defer model.Close()
//
//	result, _ := model.Choice("Classify: 'I love it!'", []string{"Positive", "Negative", "Neutral"})
//	fmt.Println(result) // Positive
//
// [Outlines]: https://github.com/dottxt-ai/outlines
package outlines

import (
	"encoding/json"
	"fmt"

	"github.com/i2y/pyffi"
)

// Model wraps an Outlines model for structured generation.
type Model struct {
	rt    *pyffi.Runtime
	model *pyffi.Object
	owns  bool
}

// NewOpenAI creates a model backed by OpenAI's API.
// Uses OPENAI_API_KEY environment variable by default.
func NewOpenAI(modelName string) (*Model, error) {
	rt, err := pyffi.New(pyffi.Dependencies("outlines", "openai"))
	if err != nil {
		return nil, fmt.Errorf("outlines: %w", err)
	}
	m, err := newOpenAI(rt, modelName)
	if err != nil {
		rt.Close()
		return nil, err
	}
	m.owns = true
	return m, nil
}

// NewOpenAIWithKey creates a model backed by OpenAI with an explicit API key.
func NewOpenAIWithKey(modelName, apiKey string) (*Model, error) {
	rt, err := pyffi.New(pyffi.Dependencies("outlines", "openai"))
	if err != nil {
		return nil, fmt.Errorf("outlines: %w", err)
	}

	code := fmt.Sprintf(`
import outlines, openai
_outlines_client = openai.OpenAI(api_key=%q)
_outlines_model = outlines.from_openai(_outlines_client, %q)
`, apiKey, modelName)

	if err := rt.Exec(code); err != nil {
		rt.Close()
		return nil, fmt.Errorf("outlines: load model: %w", err)
	}

	mainMod, _ := rt.Import("__main__")
	defer mainMod.Close()
	obj := mainMod.Attr("_outlines_model")
	if obj == nil {
		rt.Close()
		return nil, fmt.Errorf("outlines: model not found")
	}

	return &Model{rt: rt, model: obj, owns: true}, nil
}

func newOpenAI(rt *pyffi.Runtime, modelName string) (*Model, error) {
	code := fmt.Sprintf(`
import outlines, openai
_outlines_client = openai.OpenAI()
_outlines_model = outlines.from_openai(_outlines_client, %q)
`, modelName)

	if err := rt.Exec(code); err != nil {
		return nil, fmt.Errorf("outlines: load model: %w", err)
	}

	mainMod, _ := rt.Import("__main__")
	defer mainMod.Close()
	obj := mainMod.Attr("_outlines_model")
	if obj == nil {
		return nil, fmt.Errorf("outlines: model not found")
	}

	return &Model{rt: rt, model: obj}, nil
}

// NewAnthropic creates a model backed by Anthropic's API.
// Uses ANTHROPIC_API_KEY environment variable by default.
func NewAnthropic(modelName string) (*Model, error) {
	rt, err := pyffi.New(pyffi.Dependencies("outlines", "anthropic"))
	if err != nil {
		return nil, fmt.Errorf("outlines: %w", err)
	}

	code := fmt.Sprintf(`
import outlines, anthropic
_outlines_client = anthropic.Anthropic()
_outlines_model = outlines.from_anthropic(_outlines_client, %q)
`, modelName)

	if err := rt.Exec(code); err != nil {
		rt.Close()
		return nil, fmt.Errorf("outlines: load model: %w", err)
	}

	mainMod, _ := rt.Import("__main__")
	defer mainMod.Close()
	obj := mainMod.Attr("_outlines_model")
	if obj == nil {
		rt.Close()
		return nil, fmt.Errorf("outlines: model not found")
	}

	return &Model{rt: rt, model: obj, owns: true}, nil
}

// NewAnthropicWithKey creates a model backed by Anthropic with an explicit API key.
func NewAnthropicWithKey(modelName, apiKey string) (*Model, error) {
	rt, err := pyffi.New(pyffi.Dependencies("outlines", "anthropic"))
	if err != nil {
		return nil, fmt.Errorf("outlines: %w", err)
	}

	code := fmt.Sprintf(`
import outlines, anthropic
_outlines_client = anthropic.Anthropic(api_key=%q)
_outlines_model = outlines.from_anthropic(_outlines_client, %q)
`, apiKey, modelName)

	if err := rt.Exec(code); err != nil {
		rt.Close()
		return nil, fmt.Errorf("outlines: load model: %w", err)
	}

	mainMod, _ := rt.Import("__main__")
	defer mainMod.Close()
	obj := mainMod.Attr("_outlines_model")
	if obj == nil {
		rt.Close()
		return nil, fmt.Errorf("outlines: model not found")
	}

	return &Model{rt: rt, model: obj, owns: true}, nil
}

// NewOllama creates a model backed by a local Ollama server.
// Ollama must be running locally (default: http://localhost:11434).
func NewOllama(modelName string) (*Model, error) {
	rt, err := pyffi.New(pyffi.Dependencies("outlines", "ollama", "openai"))
	if err != nil {
		return nil, fmt.Errorf("outlines: %w", err)
	}

	code := fmt.Sprintf(`
import outlines, ollama
_outlines_client = ollama.Client()
_outlines_model = outlines.from_ollama(_outlines_client, %q)
`, modelName)

	if err := rt.Exec(code); err != nil {
		rt.Close()
		return nil, fmt.Errorf("outlines: load ollama model %q: %w", modelName, err)
	}

	mainMod, _ := rt.Import("__main__")
	defer mainMod.Close()
	obj := mainMod.Attr("_outlines_model")
	if obj == nil {
		rt.Close()
		return nil, fmt.Errorf("outlines: model not found")
	}

	return &Model{rt: rt, model: obj, owns: true}, nil
}

// NewTransformers creates a model from a Hugging Face transformers model.
// The model runs locally — no API key needed.
func NewTransformers(modelName string) (*Model, error) {
	rt, err := pyffi.New(pyffi.Dependencies("outlines", "transformers", "torch"))
	if err != nil {
		return nil, fmt.Errorf("outlines: %w", err)
	}

	code := fmt.Sprintf(`
import outlines
from transformers import AutoModelForCausalLM, AutoTokenizer
_outlines_hf_model = AutoModelForCausalLM.from_pretrained(%q, device_map="auto")
_outlines_hf_tokenizer = AutoTokenizer.from_pretrained(%q)
_outlines_model = outlines.from_transformers(_outlines_hf_model, _outlines_hf_tokenizer)
`, modelName, modelName)

	if err := rt.Exec(code); err != nil {
		rt.Close()
		return nil, fmt.Errorf("outlines: load model %q: %w", modelName, err)
	}

	mainMod, _ := rt.Import("__main__")
	defer mainMod.Close()
	obj := mainMod.Attr("_outlines_model")
	if obj == nil {
		rt.Close()
		return nil, fmt.Errorf("outlines: model not found")
	}

	return &Model{rt: rt, model: obj, owns: true}, nil
}

// Close releases the model and (if created by New*) the Runtime.
func (m *Model) Close() error {
	m.model.Close()
	if m.owns {
		return m.rt.Close()
	}
	return nil
}

// --- Generation Methods ---

// Choice generates one of the given choices (Literal type constraint).
func (m *Model) Choice(prompt string, choices []string) (string, error) {
	choicesJSON, _ := json.Marshal(choices)

	code := fmt.Sprintf(`
from typing import Literal
_outlines_choices = %s
_outlines_result = _outlines_model(_outlines_prompt, Literal[tuple(_outlines_choices)])
`, string(choicesJSON))

	mainMod, _ := m.rt.Import("__main__")
	defer mainMod.Close()

	promptObj := m.rt.FromString(prompt)
	defer promptObj.Close()
	mainMod.SetAttr("_outlines_prompt", promptObj)

	if err := m.rt.Exec(code); err != nil {
		return "", fmt.Errorf("outlines: choice: %w", err)
	}

	resultObj := mainMod.Attr("_outlines_result")
	if resultObj == nil {
		return "", fmt.Errorf("outlines: no result")
	}
	defer resultObj.Close()

	s, _ := resultObj.GoString()
	return s, nil
}

// JSON generates structured JSON output matching the given JSON schema.
// Returns the raw JSON string.
func (m *Model) JSON(prompt string, schema map[string]any) (string, error) {
	schemaJSON, _ := json.Marshal(schema)

	code := fmt.Sprintf(`
import json as _json
_outlines_schema = _json.loads(%q)
_outlines_result = _outlines_model(_outlines_prompt, _outlines_schema)
`, string(schemaJSON))

	mainMod, _ := m.rt.Import("__main__")
	defer mainMod.Close()

	promptObj := m.rt.FromString(prompt)
	defer promptObj.Close()
	mainMod.SetAttr("_outlines_prompt", promptObj)

	if err := m.rt.Exec(code); err != nil {
		return "", fmt.Errorf("outlines: json: %w", err)
	}

	resultObj := mainMod.Attr("_outlines_result")
	if resultObj == nil {
		return "", fmt.Errorf("outlines: no result")
	}
	defer resultObj.Close()

	s, _ := resultObj.GoString()
	return s, nil
}

// JSONTo generates structured JSON output and unmarshals it into the given Go value.
func (m *Model) JSONTo(prompt string, schema map[string]any, dest any) error {
	s, err := m.JSON(prompt, schema)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(s), dest)
}

// PydanticJSON generates structured output using a dynamically created Pydantic model.
// This works with API providers (OpenAI, Anthropic) that support tool-based structured output.
// fields maps field names to Python type strings (e.g. "str", "float", "int", "list[str]").
func (m *Model) PydanticJSON(prompt string, modelName string, fields map[string]string) (string, error) {
	var fieldDefs []string
	for name, typ := range fields {
		fieldDefs = append(fieldDefs, fmt.Sprintf("    %s: %s", name, typ))
	}

	code := fmt.Sprintf(`
from pydantic import BaseModel

class %s(BaseModel):
%s

_outlines_raw = _outlines_model(_outlines_prompt, %s)
_outlines_result = %s.model_validate_json(_outlines_raw).model_dump_json() if isinstance(_outlines_raw, str) else _outlines_raw.model_dump_json()
`, modelName, joinLines(fieldDefs), modelName, modelName)

	mainMod, _ := m.rt.Import("__main__")
	defer mainMod.Close()

	promptObj := m.rt.FromString(prompt)
	defer promptObj.Close()
	mainMod.SetAttr("_outlines_prompt", promptObj)

	if err := m.rt.Exec(code); err != nil {
		return "", fmt.Errorf("outlines: pydantic: %w", err)
	}

	resultObj := mainMod.Attr("_outlines_result")
	if resultObj == nil {
		return "", fmt.Errorf("outlines: no result")
	}
	defer resultObj.Close()

	s, _ := resultObj.GoString()
	return s, nil
}

// PydanticJSONTo generates structured output and unmarshals into a Go value.
func (m *Model) PydanticJSONTo(prompt string, modelName string, fields map[string]string, dest any) error {
	s, err := m.PydanticJSON(prompt, modelName, fields)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(s), dest)
}

func joinLines(lines []string) string {
	result := ""
	for i, l := range lines {
		if i > 0 {
			result += "\n"
		}
		result += l
	}
	if len(lines) == 0 {
		result = "    pass"
	}
	return result
}

// Regex generates text matching the given regular expression pattern.
func (m *Model) Regex(prompt string, pattern string) (string, error) {
	code := fmt.Sprintf(`
_outlines_result = _outlines_model(_outlines_prompt, %q)
`, pattern)

	mainMod, _ := m.rt.Import("__main__")
	defer mainMod.Close()

	promptObj := m.rt.FromString(prompt)
	defer promptObj.Close()
	mainMod.SetAttr("_outlines_prompt", promptObj)

	if err := m.rt.Exec(code); err != nil {
		return "", fmt.Errorf("outlines: regex: %w", err)
	}

	resultObj := mainMod.Attr("_outlines_result")
	if resultObj == nil {
		return "", fmt.Errorf("outlines: no result")
	}
	defer resultObj.Close()

	s, _ := resultObj.GoString()
	return s, nil
}

// Text generates unconstrained text (no structural constraint).
func (m *Model) Text(prompt string) (string, error) {
	code := `_outlines_result = _outlines_model(_outlines_prompt)`

	mainMod, _ := m.rt.Import("__main__")
	defer mainMod.Close()

	promptObj := m.rt.FromString(prompt)
	defer promptObj.Close()
	mainMod.SetAttr("_outlines_prompt", promptObj)

	if err := m.rt.Exec(code); err != nil {
		return "", fmt.Errorf("outlines: text: %w", err)
	}

	resultObj := mainMod.Attr("_outlines_result")
	if resultObj == nil {
		return "", fmt.Errorf("outlines: no result")
	}
	defer resultObj.Close()

	s, _ := resultObj.GoString()
	return s, nil
}
