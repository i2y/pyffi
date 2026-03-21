# casdk

Go wrapper for the [Claude Agent SDK](https://github.com/anthropics/claude-agent-sdk-python) — powered by [pyffi](https://github.com/i2y/pyffi).

The SDK is auto-installed via uv on first use. No manual Python setup required beyond having Python 3.12+ available.

## Install

```bash
go get github.com/i2y/pyffi/casdk
```

Set `ANTHROPIC_API_KEY` for queries:

```bash
export ANTHROPIC_API_KEY=sk-ant-...
```

## One-Off Query

```go
client, _ := casdk.New()
defer client.Close()

for msg, err := range client.Query(ctx, "Explain Go interfaces",
    casdk.WithMaxTurns(1),
    casdk.WithModel("sonnet"),
) {
    if err != nil { log.Fatal(err) }
    defer msg.Close()

    if msg.Type() == "assistant" {
        for _, block := range msg.ContentBlocks() {
            if tb, ok := block.(casdk.TextBlock); ok {
                fmt.Println(tb.Text)
            }
        }
    }
    if msg.Type() == "result" {
        fmt.Printf("Cost: $%.4f, Turns: %d\n", msg.TotalCostUSD(), msg.NumTurns())
    }
}
```

## Interactive Session

Multi-turn conversations with context retained between queries. Use `client.Session()` to share the Runtime with the client (recommended), or `casdk.NewSession()` for a standalone session:

```go
client, _ := casdk.New()
defer client.Close()

// Create session from existing client (shares Runtime — recommended)
session, _ := client.Session(
    casdk.WithModel("sonnet"),
    casdk.WithCwd("/path/to/project"),
)
defer session.Close()

session.Query("What files are in this directory?")
for msg, _ := range session.ReceiveMessages() {
    defer msg.Close()
    fmt.Println(msg.Text())
}

// Follow-up — context is retained
session.Query("Now explain the main.go file")
for msg, _ := range session.ReceiveMessages() {
    defer msg.Close()
    fmt.Println(msg.Text())
}
```

## Session Management

List and inspect past Claude Code sessions (no API key needed):

```go
client, _ := casdk.New()
defer client.Close()

sessions, _ := client.ListSessions(casdk.WithLimit(10))
for _, s := range sessions {
    fmt.Printf("%s  %s\n", s.SessionID[:8], s.Summary)
}

msgs, _ := client.GetSessionMessages(sessions[0].SessionID)
for _, m := range msgs {
    fmt.Printf("[%s] %s\n", m.Type, m.Content)
}

client.RenameSession(sessions[0].SessionID, "My Session")
client.TagSession(sessions[0].SessionID, "important")
```

## Query Options

```go
casdk.WithModel("sonnet")                   // model selection
casdk.WithSystemPrompt("Be concise")        // custom system prompt
casdk.WithMaxTurns(5)                       // limit agentic turns
casdk.WithMaxBudget(1.0)                    // USD budget cap
casdk.WithPermissionMode("bypassPermissions") // permission mode
casdk.WithCwd("/path/to/project")           // working directory
casdk.WithAllowedTools("Read", "Bash")      // auto-approve tools
casdk.WithDisallowedTools("Write")          // block tools
casdk.WithResume("session-id")              // resume session
casdk.WithCLIPath("/path/to/claude")        // custom CLI path
casdk.WithThinking(casdk.ThinkingConfig{    // extended thinking
    Type: "enabled", BudgetTokens: 10000,
})
casdk.WithSandbox(casdk.SandboxConfig{      // sandbox mode
    Enabled: true,
    AutoAllowBashIfSandboxed: true,
})
casdk.WithMCPServers(map[string]casdk.MCPServerConfig{
    "myserver": {Command: "node", Args: []string{"server.js"}},
})
casdk.WithAgents(map[string]casdk.AgentDefinition{
    "reviewer": {
        Description: "Code reviewer",
        Prompt:      "Review code quality...",
        Tools:       []string{"Read", "Glob", "Grep"},
    },
})
```

## Plugins

Load local SDK plugins:

```go
client.Query(ctx, "Hello",
    casdk.WithPlugins(casdk.PluginConfig{Type: "local", Path: "/path/to/plugin"}),
)
```

## Custom Tool Permissions

Use `WithCanUseTool` to control which tools Claude can use at runtime:

```go
client.Query(ctx, "Refactor main.go",
    casdk.WithCanUseTool(func(toolName string, input map[string]any) casdk.PermissionResult {
        if toolName == "Bash" {
            return casdk.PermissionResult{Message: "Bash is not allowed"}
        }
        return casdk.PermissionResult{Allow: true}
    }),
)
```

## Hooks

Register Go callbacks for lifecycle events:

```go
client.Query(ctx, "Fix the bug",
    casdk.WithHooks(map[casdk.HookEvent][]casdk.HookMatcher{
        casdk.HookPreToolUse: {
            {
                Matcher: "Bash",
                Handler: func(input map[string]any) (map[string]any, error) {
                    fmt.Println("About to run Bash:", input)
                    return nil, nil // allow
                },
            },
        },
    }),
)
```

## In-Process MCP Servers

Create MCP servers with Go tool handlers that run in the same process:

```go
client.Query(ctx, "What is 2+3?",
    casdk.WithSDKMCPServers(casdk.SDKMCPServer{
        Name: "calculator",
        Tools: []casdk.MCPTool{
            {
                Name:        "add",
                Description: "Add two numbers",
                InputSchema: map[string]any{
                    "type": "object",
                    "properties": map[string]any{
                        "a": map[string]any{"type": "number"},
                        "b": map[string]any{"type": "number"},
                    },
                },
                Handler: func(args map[string]any) (string, error) {
                    a, _ := args["a"].(float64)
                    b, _ := args["b"].(float64)
                    return fmt.Sprintf("%g", a+b), nil
                },
            },
        },
    }),
    casdk.WithAllowedTools("mcp__calculator__add"),
)
```

## Message Types

| Type | Description | Key Fields |
|------|-------------|------------|
| `"user"` | User input | `Text()` |
| `"assistant"` | Claude's response | `Text()`, `Model()`, `ContentBlocks()` |
| `"system"` | System events | `SessionID()` |
| `"result"` | Final result | `Text()`, `IsError()`, `TotalCostUSD()`, `Usage()`, `NumTurns()`, `DurationMs()` |

### Content Blocks

Assistant messages contain typed content blocks:

```go
for _, block := range msg.ContentBlocks() {
    switch b := block.(type) {
    case casdk.TextBlock:
        fmt.Println(b.Text)
    case casdk.ToolUseBlock:
        fmt.Printf("Tool: %s(%v)\n", b.Name, b.Input)
    case casdk.ThinkingBlock:
        fmt.Println("Thinking:", b.Thinking)
    case casdk.ToolResultBlock:
        fmt.Printf("Result: %s (error=%v)\n", b.Content, b.IsError)
    }
}
```

## For LLM Agents

- [`skills/casdk/SKILL.md`](skills/casdk/SKILL.md) — AgentSkills-format usage guide for AI coding assistants
