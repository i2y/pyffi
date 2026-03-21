# oasdk

> **Note:** This package exists as a demonstration of wrapping a Python SDK with pyffi. In practice, the OpenAI Agents SDK's core logic (LLM API calls + agent loop) can be reimplemented in pure Go without a Python dependency, and native Go alternatives like [openai-agents-go](https://github.com/MitulShah1/openai-agents-go) or [swarmgo](https://github.com/prathyushnallamothu/swarmgo) may be more practical for production use. For a better example of pyffi's value — wrapping a library that **cannot** be reimplemented in Go — see [sbert](../sbert/) (sentence-transformers) or [casdk](../casdk/) (Claude Agent SDK with CLI subprocess protocol).

Go wrapper for the [OpenAI Agents SDK](https://github.com/openai/openai-agents-python) — powered by [pyffi](https://github.com/i2y/pyffi).

The SDK is auto-installed via uv on first use. No manual Python setup required beyond having Python 3.12+ available.

## Install

```bash
go get github.com/i2y/pyffi/oasdk
```

Set `OPENAI_API_KEY` for queries:

```bash
export OPENAI_API_KEY=sk-...
```

## Quick Start

```go
client, _ := oasdk.New()
defer client.Close()

agent := oasdk.NewAgent("Assistant",
    oasdk.WithInstructions("You are a helpful assistant"),
)

result, _ := client.Run(agent, "What is 2+2?")
fmt.Println(result.FinalOutput())
```

## Agent Options

```go
oasdk.WithInstructions("You are helpful")        // system prompt
oasdk.WithModel("gpt-4o")                        // model selection
oasdk.WithModelSettings(oasdk.ModelSettings{      // tuning parameters
    Temperature: ptr(0.7),
})
oasdk.WithTools(tool1, tool2)                     // function tools
oasdk.WithHandoffs(agentA, agentB)                // delegate to other agents
oasdk.WithToolUseBehavior("stop_on_first_tool")   // stop after first tool call
```

## Run Options

```go
oasdk.WithMaxTurns(5)                             // limit agentic turns
oasdk.WithModelOverride("gpt-4o-mini")            // override model for this run
oasdk.WithPreviousResult(result)                  // multi-turn conversation
```

## Tools

Define Go functions as tools that the agent can call:

```go
agent := oasdk.NewAgent("Assistant",
    oasdk.WithInstructions("Use the add tool when asked to add numbers"),
    oasdk.WithTools(
        oasdk.FuncTool("add", "Add two numbers", map[string]any{
            "type": "object",
            "properties": map[string]any{
                "a": map[string]any{"type": "number"},
                "b": map[string]any{"type": "number"},
            },
            "required": []string{"a", "b"},
        }, func(args map[string]any) (string, error) {
            a, _ := args["a"].(float64)
            b, _ := args["b"].(float64)
            return fmt.Sprintf("%g", a+b), nil
        }),
    ),
)

result, _ := client.Run(agent, "What is 7+3?")
fmt.Println(result.FinalOutput()) // 10
```

## Handoffs

Delegate to specialist agents:

```go
booking := oasdk.NewAgent("Booking",
    oasdk.WithInstructions("Handle booking requests"),
)

refund := oasdk.NewAgent("Refund",
    oasdk.WithInstructions("Handle refund requests"),
)

triage := oasdk.NewAgent("Triage",
    oasdk.WithInstructions("Route to the appropriate specialist"),
    oasdk.WithHandoffs(booking, refund),
)

result, _ := client.Run(triage, "I want to book a flight")
fmt.Println(result.LastAgent()) // "Booking"
```

## Multi-Turn Conversations

```go
result1, _ := client.Run(agent, "What's the weather in Tokyo?")
fmt.Println(result1.FinalOutput())

result2, _ := client.Run(agent, "And in Osaka?",
    oasdk.WithPreviousResult(result1),
)
fmt.Println(result2.FinalOutput())
```

## RunResult

| Method | Description |
|--------|-------------|
| `FinalOutput()` | Final text output from the agent |
| `LastAgent()` | Name of the agent that produced the output |
| `Items()` | Run items (messages, tool calls, tool results, handoffs) |
| `InputList()` | Conversation history for multi-turn use |
