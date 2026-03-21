// Package smolagentsgo provides Go bindings for [smolagents], Hugging Face's
// lightweight agent framework. Build agents that write Python code to
// orchestrate tools and solve multi-step tasks.
//
// The Python packages are auto-installed via uv on first use.
//
//	client, _ := smolagentsgo.New(smolagentsgo.WithLiteLLM("anthropic/claude-3-haiku-20240307", key))
//	defer client.Close()
//
//	result, _ := client.Run("What is the current Go version?", smolagentsgo.WithTools("web_search"))
//	fmt.Println(result)
//
// [smolagents]: https://github.com/huggingface/smolagents
package smolagentsgo

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/i2y/pyffi"
)

var callbackSeq atomic.Int64

func nextCallbackID(prefix string) string {
	return fmt.Sprintf("_smol_%s_%d", prefix, callbackSeq.Add(1))
}

// Client wraps a smolagents agent.
type Client struct {
	rt *pyffi.Runtime
}

// ClientOption configures a Client.
type ClientOption func(*clientConfig)

type clientConfig struct {
	modelType string // "inference", "litellm", "openai"
	modelID   string
	apiKey    string
	apiBase   string
	agentType string // "code" (default), "tool_calling"
}

// WithInferenceClient uses Hugging Face Inference API.
func WithInferenceClient(modelID string) ClientOption {
	return func(c *clientConfig) { c.modelType = "inference"; c.modelID = modelID }
}

// WithLiteLLM uses LiteLLM (supports 100+ providers including Anthropic, OpenAI).
func WithLiteLLM(modelID, apiKey string) ClientOption {
	return func(c *clientConfig) { c.modelType = "litellm"; c.modelID = modelID; c.apiKey = apiKey }
}

// WithOpenAI uses an OpenAI-compatible API.
func WithOpenAI(modelID, apiKey string) ClientOption {
	return func(c *clientConfig) { c.modelType = "openai"; c.modelID = modelID; c.apiKey = apiKey }
}

// WithOpenAIBase uses an OpenAI-compatible API with a custom base URL.
func WithOpenAIBase(modelID, apiKey, apiBase string) ClientOption {
	return func(c *clientConfig) {
		c.modelType = "openai"
		c.modelID = modelID
		c.apiKey = apiKey
		c.apiBase = apiBase
	}
}

// WithAgentType sets the agent type: "code" (default) or "tool_calling".
func WithAgentType(t string) ClientOption {
	return func(c *clientConfig) { c.agentType = t }
}

// New creates a new smolagents client.
func New(opts ...ClientOption) (*Client, error) {
	cfg := clientConfig{modelType: "inference", agentType: "code"}
	for _, o := range opts {
		o(&cfg)
	}

	rt, err := pyffi.New(pyffi.Dependencies("smolagents[toolkit,litellm]"))
	if err != nil {
		return nil, fmt.Errorf("smolagentsgo: %w", err)
	}

	// Build model code
	var modelCode string
	switch cfg.modelType {
	case "inference":
		if cfg.modelID != "" {
			modelCode = fmt.Sprintf("_smol_model = smolagents.InferenceClientModel(model_id=%q)", cfg.modelID)
		} else {
			modelCode = "_smol_model = smolagents.InferenceClientModel()"
		}
	case "litellm":
		parts := []string{fmt.Sprintf("model_id=%q", cfg.modelID)}
		if cfg.apiKey != "" {
			parts = append(parts, fmt.Sprintf("api_key=%q", cfg.apiKey))
		}
		modelCode = fmt.Sprintf("_smol_model = smolagents.LiteLLMModel(%s)", strings.Join(parts, ", "))
	case "openai":
		parts := []string{fmt.Sprintf("model_id=%q", cfg.modelID)}
		if cfg.apiKey != "" {
			parts = append(parts, fmt.Sprintf("api_key=%q", cfg.apiKey))
		}
		if cfg.apiBase != "" {
			parts = append(parts, fmt.Sprintf("api_base=%q", cfg.apiBase))
		}
		modelCode = fmt.Sprintf("_smol_model = smolagents.OpenAIModel(%s)", strings.Join(parts, ", "))
	}

	code := fmt.Sprintf("import smolagents\n%s\n", modelCode)
	if err := rt.Exec(code); err != nil {
		rt.Close()
		return nil, fmt.Errorf("smolagentsgo: configure model: %w", err)
	}

	return &Client{rt: rt}, nil
}

// Close releases all resources.
func (c *Client) Close() error {
	return c.rt.Close()
}

// Runtime returns the underlying pyffi Runtime.
func (c *Client) Runtime() *pyffi.Runtime { return c.rt }

// RunOption configures a Run call.
type RunOption func(*runConfig)

type runConfig struct {
	tools       []string // built-in tool names
	customTools []Tool
	agentType   string // override agent type for this run
	maxSteps    int
}

// WithTools adds built-in tools by name (e.g. "web_search").
func WithTools(names ...string) RunOption {
	return func(c *runConfig) { c.tools = append(c.tools, names...) }
}

// WithCustomTool adds a Go function as a tool.
func WithCustomTool(tool Tool) RunOption {
	return func(c *runConfig) { c.customTools = append(c.customTools, tool) }
}

// WithMaxSteps limits the number of agent steps.
func WithMaxSteps(n int) RunOption {
	return func(c *runConfig) { c.maxSteps = n }
}

// Tool defines a custom tool backed by a Go function.
type Tool struct {
	Name        string
	Description string
	Handler     func(args map[string]any) (string, error)
	Inputs      map[string]ToolInput // parameter definitions
}

// ToolInput describes a tool parameter.
type ToolInput struct {
	Type        string // "string", "integer", "number", "boolean"
	Description string
}

// Run executes an agent task and returns the result.
func (c *Client) Run(task string, opts ...RunOption) (string, error) {
	var cfg runConfig
	for _, o := range opts {
		o(&cfg)
	}

	var code strings.Builder
	code.WriteString("import smolagents\n")

	// Built-in tools
	var toolRefs []string
	for _, name := range cfg.tools {
		switch name {
		case "web_search":
			toolRefs = append(toolRefs, "smolagents.WebSearchTool()")
		default:
			toolRefs = append(toolRefs, fmt.Sprintf("smolagents.load_tool(%q)", name))
		}
	}

	// Custom tools via RegisterFunc
	for i, tool := range cfg.customTools {
		cbName := nextCallbackID(fmt.Sprintf("tool_%d", i))
		handler := tool.Handler
		if err := c.rt.RegisterFunc(cbName, func(args map[string]any) (string, error) {
			return handler(args)
		}); err != nil {
			return "", fmt.Errorf("smolagentsgo: register tool %s: %w", tool.Name, err)
		}

		// Build inputs dict
		inputsJSON, _ := json.Marshal(tool.Inputs)

		code.WriteString(fmt.Sprintf(`
import go_bridge, json

class _SmolTool_%s(smolagents.Tool):
    name = %q
    description = %q
    inputs = {k: {"type": v["Type"], "description": v["Description"]} for k, v in json.loads(%q).items()}
    output_type = "string"

    def forward(self, **kwargs):
        return go_bridge.%s(kwargs)

`, cbName, tool.Name, tool.Description, string(inputsJSON), cbName))
		toolRefs = append(toolRefs, fmt.Sprintf("_SmolTool_%s()", cbName))
	}

	// Agent type
	agentType := cfg.agentType
	if agentType == "" {
		agentType = "code"
	}
	agentClass := "CodeAgent"
	if agentType == "tool_calling" {
		agentClass = "ToolCallingAgent"
	}

	var agentParts []string
	agentParts = append(agentParts, fmt.Sprintf("tools=[%s]", strings.Join(toolRefs, ", ")))
	agentParts = append(agentParts, "model=_smol_model")
	if cfg.maxSteps > 0 {
		agentParts = append(agentParts, fmt.Sprintf("max_steps=%d", cfg.maxSteps))
	}

	code.WriteString(fmt.Sprintf("_smol_agent = smolagents.%s(%s)\n", agentClass, strings.Join(agentParts, ", ")))

	// Set task
	mainMod, _ := c.rt.Import("__main__")
	defer mainMod.Close()
	taskObj := c.rt.FromString(task)
	defer taskObj.Close()
	mainMod.SetAttr("_smol_task", taskObj)

	code.WriteString("_smol_result = str(_smol_agent.run(_smol_task))\n")

	if err := c.rt.Exec(code.String()); err != nil {
		return "", fmt.Errorf("smolagentsgo: run: %w", err)
	}

	resultObj := mainMod.Attr("_smol_result")
	if resultObj == nil {
		return "", fmt.Errorf("smolagentsgo: no result")
	}
	defer resultObj.Close()
	return resultObj.GoString()
}
