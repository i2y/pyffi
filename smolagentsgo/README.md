# smolagentsgo

Go bindings for [smolagents](https://github.com/huggingface/smolagents) — powered by [pyffi](https://github.com/i2y/pyffi).

smolagents is Hugging Face's lightweight agent framework. Agents write Python code to orchestrate tools and solve multi-step tasks. This package lets you build and run agents from Go, including custom Go-backed tools.

## Install

```bash
go get github.com/i2y/pyffi/smolagentsgo
```

Python 3.12+ is required at runtime. The `smolagents` package is auto-installed via uv on first use.

## Quick Start

```go
client, _ := smolagentsgo.New(
    smolagentsgo.WithLiteLLM("anthropic/claude-3-haiku-20240307", os.Getenv("ANTHROPIC_API_KEY")),
)
defer client.Close()

result, _ := client.Run("What is 2+2?")
fmt.Println(result) // 4
```

## Built-in Tools

```go
result, _ := client.Run("Search for the latest Go version",
    smolagentsgo.WithTools("web_search"),
)
```

## Custom Go Tools

Register Go functions as tools that the agent can call:

```go
result, _ := client.Run("What's the weather in Tokyo?",
    smolagentsgo.WithCustomTool(smolagentsgo.Tool{
        Name:        "get_weather",
        Description: "Get weather for a city",
        Inputs: map[string]smolagentsgo.ToolInput{
            "city": {Type: "string", Description: "City name"},
        },
        Handler: func(args map[string]any) (string, error) {
            city := args["city"].(string)
            return fmt.Sprintf("Weather in %s: sunny, 22°C", city), nil
        },
    }),
)
```

## Model Providers

```go
// Hugging Face Inference API
smolagentsgo.WithInferenceClient("meta-llama/Llama-3-8B-Instruct")

// LiteLLM (100+ providers: Anthropic, OpenAI, etc.)
smolagentsgo.WithLiteLLM("anthropic/claude-3-haiku-20240307", apiKey)

// OpenAI-compatible API
smolagentsgo.WithOpenAI("gpt-4o", apiKey)

// Custom endpoint
smolagentsgo.WithOpenAIBase("deepseek-ai/DeepSeek-R1", apiKey, "https://api.together.xyz/v1/")
```

## Run Options

```go
smolagentsgo.WithTools("web_search")        // built-in tools
smolagentsgo.WithCustomTool(tool)           // Go-backed custom tool
smolagentsgo.WithMaxSteps(10)               // limit agent steps
```

## Agent Types

By default, smolagentsgo uses `CodeAgent` which writes Python code for actions. For JSON-based tool calling:

```go
client, _ := smolagentsgo.New(
    smolagentsgo.WithLiteLLM("anthropic/claude-3-haiku-20240307", key),
    smolagentsgo.WithAgentType("tool_calling"),
)
```

## API Reference

| Method | Description |
|--------|-------------|
| `New(opts...)` | Create agent client |
| `client.Run(task, opts...)` | Run agent task |
| `client.Close()` | Release resources |
