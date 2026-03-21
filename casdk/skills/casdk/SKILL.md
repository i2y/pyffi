---
name: casdk
description: "Guide for writing Go code using casdk, a Go wrapper for the Claude Agent SDK. Use this skill when building Go applications that interact with Claude Code — sending queries, managing sessions, listing past conversations, or using Claude as an agentic coding assistant from Go. Also use when the user mentions Claude Agent SDK from Go, claude-agent-sdk Go bindings, Go + Claude Code automation, or programmatic Claude Code access."
license: MIT
compatibility: "Requires Python 3.12+ and ANTHROPIC_API_KEY for queries. The SDK is auto-installed via uv on first use."
metadata:
  author: Yasushi Itoh
  version: "1.0"
---

# casdk — Claude Agent SDK for Go

casdk is a Go wrapper around the Claude Agent SDK, powered by pyffi (Go → Python via purego, no Cgo). The Python SDK is auto-installed via uv on first use.

For full API reference, read `README.md` in the casdk package root.

## Setup

```go
import "github.com/i2y/pyffi/casdk"
```

Set `ANTHROPIC_API_KEY` for queries. Session listing does not require an API key.

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

    switch msg.Type() {
    case "assistant":
        for _, block := range msg.ContentBlocks() {
            if tb, ok := block.(casdk.TextBlock); ok {
                fmt.Println(tb.Text)
            }
        }
    case "result":
        fmt.Printf("Cost: $%.4f, Turns: %d\n", msg.TotalCostUSD(), msg.NumTurns())
    }
}
```

## Interactive Session

Multi-turn conversations with retained context. Use `client.Session()` to share the Runtime (recommended):

```go
client, _ := casdk.New()
defer client.Close()

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

session.Query("Now explain the main.go file")
for msg, _ := range session.ReceiveMessages() {
    defer msg.Close()
    fmt.Println(msg.Text())
}
```

## Session Management (No API Key Needed)

```go
client, _ := casdk.New()
defer client.Close()

// List sessions
sessions, _ := client.ListSessions(casdk.WithLimit(10))
for _, s := range sessions {
    fmt.Printf("%s  %s\n", s.SessionID[:8], s.Summary)
}

// Get messages
msgs, _ := client.GetSessionMessages(sessions[0].SessionID)
for _, m := range msgs {
    fmt.Printf("[%s] %s\n", m.Type, m.Content)
}

// Manage
client.RenameSession(sessions[0].SessionID, "My Session")
client.TagSession(sessions[0].SessionID, "important")
```

## All Query Options

```go
casdk.WithModel("sonnet")                    // model selection
casdk.WithSystemPrompt("Be concise")         // custom system prompt
casdk.WithMaxTurns(5)                        // limit agentic turns
casdk.WithMaxBudget(1.0)                     // USD budget cap
casdk.WithPermissionMode("bypassPermissions") // "default", "acceptEdits", "plan", "bypassPermissions"
casdk.WithCwd("/path/to/project")            // working directory
casdk.WithAllowedTools("Read", "Bash")       // auto-approve tools
casdk.WithDisallowedTools("Write")           // block tools
casdk.WithResume("session-id")               // resume existing session
casdk.WithCLIPath("/path/to/claude")         // custom CLI path
casdk.WithIncludePartialMessages(true)       // streaming partial updates
casdk.WithEnableFileCheckpointing(true)      // file change tracking
casdk.WithSettingSources("user", "project")  // load settings from sources
casdk.WithThinking(casdk.ThinkingConfig{     // extended thinking
    Type: "enabled", BudgetTokens: 10000,
})
casdk.WithSandbox(casdk.SandboxConfig{       // sandbox mode
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
casdk.WithPlugins(casdk.PluginConfig{           // local SDK plugins
    Type: "local", Path: "/path/to/plugin",
})
casdk.WithCanUseTool(func(toolName string,      // custom tool permissions
    input map[string]any) casdk.PermissionResult {
    return casdk.PermissionResult{Allow: true}
})
casdk.WithHooks(map[casdk.HookEvent][]casdk.HookMatcher{ // lifecycle hooks
    casdk.HookPreToolUse: {{
        Matcher: "Bash",
        Handler: func(input map[string]any) (map[string]any, error) {
            return nil, nil
        },
    }},
})
casdk.WithSDKMCPServers(casdk.SDKMCPServer{     // in-process MCP servers
    Name: "myserver",
    Tools: []casdk.MCPTool{{
        Name: "greet", Description: "Say hello",
        InputSchema: map[string]any{"type": "object",
            "properties": map[string]any{"name": map[string]any{"type": "string"}}},
        Handler: func(args map[string]any) (string, error) {
            return fmt.Sprintf("Hello, %s!", args["name"]), nil
        },
    }},
})
```

## Message Types

| Type | Description | Key Methods |
|------|-------------|-------------|
| `"user"` | User input | `Text()` |
| `"assistant"` | Claude's response | `Text()`, `Model()`, `ContentBlocks()` |
| `"system"` | System events | `SessionID()` |
| `"result"` | Final result | `Text()`, `IsError()`, `TotalCostUSD()`, `Usage()`, `NumTurns()`, `DurationMs()` |

## Content Blocks

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

## Usage (Token Info)

```go
if u := msg.Usage(); u != nil {
    fmt.Printf("In: %d, Out: %d, Cache: %d\n",
        u.InputTokens, u.OutputTokens, u.CacheReadInputTokens)
}
```

## Critical Rules

1. **Always `defer client.Close()` and `defer msg.Close()`** — resource leaks otherwise.
2. **`iter.Seq2` pattern** — `Query` and `ReceiveMessages` return Go 1.23+ range iterators.
3. **`ANTHROPIC_API_KEY`** is required for `Query` and `Session`, but NOT for `ListSessions` / `GetSessionMessages`.
4. **The SDK auto-installs** via uv on first `casdk.New()` call. No manual `pip install` needed.
