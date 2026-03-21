// Package oasdk provides a Go-idiomatic wrapper around the
// [OpenAI Agents SDK] for building multi-agent workflows.
//
// The SDK is auto-installed via uv on first use. Set the OPENAI_API_KEY
// environment variable for authentication.
//
//	client, _ := oasdk.New()
//	defer client.Close()
//
//	agent := oasdk.NewAgent("Assistant",
//	    oasdk.WithInstructions("You are helpful"),
//	)
//	result, _ := client.Run(agent, "Hello")
//	fmt.Println(result.FinalOutput())
//
// [OpenAI Agents SDK]: https://github.com/openai/openai-agents-python
package oasdk

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/i2y/pyffi"
)

// callbackSeq generates unique names for Go→Python callbacks.
var callbackSeq atomic.Int64

func nextCallbackID(prefix string) string {
	return fmt.Sprintf("_oasdk_%s_%d", prefix, callbackSeq.Add(1))
}

// Client wraps the OpenAI Agents SDK.
type Client struct {
	rt *pyffi.Runtime
}

// New creates a new OpenAI Agents SDK client.
// The SDK is auto-installed via uv if not present.
func New() (*Client, error) {
	rt, err := pyffi.New(pyffi.Dependencies("openai-agents"))
	if err != nil {
		return nil, fmt.Errorf("oasdk: %w", err)
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

// Run executes an agent workflow and returns the result.
func (c *Client) Run(agent *Agent, input string, opts ...RunOption) (*RunResult, error) {
	var cfg runConfig
	for _, o := range opts {
		o(&cfg)
	}

	code, err := c.buildRunCode(agent, input, cfg)
	if err != nil {
		return nil, err
	}

	mainMod, _ := c.rt.Import("__main__")
	defer mainMod.Close()

	// Set input prompt
	promptObj := c.rt.FromString(input)
	defer promptObj.Close()
	mainMod.SetAttr("_oasdk_prompt", promptObj)

	// Set previous input list if multi-turn
	if cfg.previousInputList != "" {
		ilObj := c.rt.FromString(cfg.previousInputList)
		defer ilObj.Close()
		mainMod.SetAttr("_oasdk_prev_input_list", ilObj)
	}

	if err := c.rt.Exec(code); err != nil {
		return nil, fmt.Errorf("oasdk: run failed: %w", err)
	}

	resultObj := mainMod.Attr("_oasdk_run_result")
	if resultObj == nil {
		return nil, fmt.Errorf("oasdk: no result")
	}

	return extractRunResult(c.rt, resultObj), nil
}

// buildRunCode generates Python code for agent creation and Runner.run_sync().
func (c *Client) buildRunCode(agent *Agent, input string, cfg runConfig) (string, error) {
	var code strings.Builder

	code.WriteString("from agents import Agent, Runner, FunctionTool\n")

	// Collect all agents in the graph (for handoffs)
	allAgents := collectAgents(agent)
	agentVars := make(map[*Agent]string)
	for i, a := range allAgents {
		agentVars[a] = fmt.Sprintf("_oasdk_agent_%d", i)
	}

	// Register tools and generate tool code.
	// toolVarNames maps (agent pointer, tool index) to Python variable name.
	type toolKey struct {
		agent *Agent
		index int
	}
	toolVarNames := make(map[toolKey]string)

	code.WriteString("import go_bridge, json as _json\n")

	for _, a := range allAgents {
		for i, tool := range a.tools {
			cbName := nextCallbackID(fmt.Sprintf("tool_%d", i))
			handler := tool.Handler
			if err := c.rt.RegisterFunc(cbName, func(args map[string]any) (string, error) {
				return handler(args)
			}); err != nil {
				return "", fmt.Errorf("oasdk: register tool %s: %w", tool.Name, err)
			}

			toolVar := cbName + "_tool"
			toolVarNames[toolKey{a, i}] = toolVar

			schemaJSON, _ := json.Marshal(tool.Schema)
			code.WriteString(fmt.Sprintf(`
async def %s_invoke(ctx, input_str):
    args = _json.loads(input_str)
    return go_bridge.%s(args)
%s = FunctionTool(name=%q, description=%q, params_json_schema=%s, on_invoke_tool=%s_invoke)
`, cbName, cbName, toolVar, tool.Name, tool.Description, string(schemaJSON), cbName))
		}
	}

	// Pass 1: create agents (without handoffs)
	for _, a := range allAgents {
		varName := agentVars[a]
		var parts []string
		parts = append(parts, fmt.Sprintf("name=%q", a.name))
		if a.instructions != "" {
			parts = append(parts, fmt.Sprintf("instructions=%q", a.instructions))
		}
		if a.model != "" {
			parts = append(parts, fmt.Sprintf("model=%q", a.model))
		}
		if a.toolUseBehavior != "" && a.toolUseBehavior != "run_llm_again" {
			parts = append(parts, fmt.Sprintf("tool_use_behavior=%q", a.toolUseBehavior))
		}
		if a.modelSettings != nil {
			msParts := []string{}
			if a.modelSettings.Temperature != nil {
				msParts = append(msParts, fmt.Sprintf("'temperature': %f", *a.modelSettings.Temperature))
			}
			if a.modelSettings.TopP != nil {
				msParts = append(msParts, fmt.Sprintf("'top_p': %f", *a.modelSettings.TopP))
			}
			if a.modelSettings.MaxTokens != nil {
				msParts = append(msParts, fmt.Sprintf("'max_tokens': %d", *a.modelSettings.MaxTokens))
			}
			parts = append(parts, fmt.Sprintf("model_settings={%s}", strings.Join(msParts, ", ")))
		}

		// Tools
		if len(a.tools) > 0 {
			var toolRefs []string
			for i := range a.tools {
				toolRefs = append(toolRefs, toolVarNames[toolKey{a, i}])
			}
			parts = append(parts, fmt.Sprintf("tools=[%s]", strings.Join(toolRefs, ", ")))
		}

		code.WriteString(fmt.Sprintf("%s = Agent(%s)\n", varName, strings.Join(parts, ", ")))
	}

	// Pass 2: set handoffs
	for _, a := range allAgents {
		if len(a.handoffs) > 0 {
			var refs []string
			for _, h := range a.handoffs {
				refs = append(refs, agentVars[h])
			}
			code.WriteString(fmt.Sprintf("%s.handoffs = [%s]\n",
				agentVars[a], strings.Join(refs, ", ")))
		}
	}

	// Run
	rootVar := agentVars[agent]
	var runParts []string
	runParts = append(runParts, rootVar)

	if cfg.previousInputList != "" {
		code.WriteString(`
_oasdk_prev = _json.loads(_oasdk_prev_input_list)
_oasdk_prev.append({"role": "user", "content": _oasdk_prompt})
`)
		runParts = append(runParts, "_oasdk_prev")
	} else {
		runParts = append(runParts, "_oasdk_prompt")
	}

	if cfg.maxTurns > 0 {
		runParts = append(runParts, fmt.Sprintf("max_turns=%d", cfg.maxTurns))
	}

	code.WriteString(fmt.Sprintf("_oasdk_run_result = Runner.run_sync(%s)\n",
		strings.Join(runParts, ", ")))

	return code.String(), nil
}

// collectAgents walks the agent graph and returns all unique agents.
func collectAgents(root *Agent) []*Agent {
	var result []*Agent
	visited := map[*Agent]bool{}

	var walk func(a *Agent)
	walk = func(a *Agent) {
		if visited[a] {
			return
		}
		visited[a] = true
		result = append(result, a)
		for _, h := range a.handoffs {
			walk(h)
		}
	}

	walk(root)
	return result
}
