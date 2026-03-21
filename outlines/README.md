# outlines

Go bindings for [Outlines](https://github.com/dottxt-ai/outlines) — powered by [pyffi](https://github.com/i2y/pyffi).

Outlines provides **structured generation** for LLMs. Instead of hoping the model produces valid output and parsing it afterwards, Outlines guarantees the output matches your specified type *during* token generation via constrained decoding.

## Install

```bash
go get github.com/i2y/pyffi/outlines
```

Python 3.12+ is required at runtime. The `outlines` package is auto-installed via uv on first use.

## Why Wrap Outlines?

Outlines' constrained decoding works at the token level using finite state machines built from regex patterns and JSON schemas. This is deeply integrated with model tokenizers and cannot be practically reimplemented in Go.

## Quick Start

```go
model, _ := outlines.NewOllama("llama3.2")
defer model.Close()

// Generate structured JSON via Pydantic model
result, _ := model.PydanticJSON(
    "Generate a profile for a Go programmer.",
    "Profile",
    map[string]string{
        "name": "str",
        "age":  "int",
    },
)
fmt.Println(result) // {"age":40,"name":"John Doe"}

// Unmarshal directly into a Go struct
var profile struct {
    Name string `json:"name"`
    Age  int    `json:"age"`
}
model.PydanticJSONTo(
    "Generate a profile for a Go programmer.",
    "Profile",
    map[string]string{"name": "str", "age": "int"},
    &profile,
)
fmt.Println(profile.Name, profile.Age)
```

## Model Providers

### Ollama (Local)

Run with a local Ollama server — no API key needed:

```go
model, _ := outlines.NewOllama("llama3.2")
```

### OpenAI (API)

```go
model, _ := outlines.NewOpenAI("gpt-4o")                  // uses OPENAI_API_KEY env
model, _ := outlines.NewOpenAIWithKey("gpt-4o", "sk-...")  // explicit key
```

### Anthropic (API)

```go
model, _ := outlines.NewAnthropic("claude-3-haiku-20240307")                  // uses ANTHROPIC_API_KEY env
model, _ := outlines.NewAnthropicWithKey("claude-3-haiku-20240307", "sk-...")  // explicit key
```

### Local Models (Transformers)

Run Hugging Face models directly — requires PyTorch:

```go
model, _ := outlines.NewTransformers("microsoft/Phi-3-mini-4k-instruct")
```

## Generation Methods

### PydanticJSON — Structured JSON (recommended)

Works with all providers. Define fields as a map of name → Python type:

```go
result, _ := model.PydanticJSON(
    "Extract the event details.",
    "Event",
    map[string]string{
        "title":    "str",
        "date":     "str",
        "location": "str",
        "attendees": "int",
    },
)
```

### Choice — Multiple Choice

Force output to one of the given strings. Works with OpenAI and local models:

```go
sentiment, _ := model.Choice(
    "Classify: 'This product changed my life!'",
    []string{"Positive", "Negative", "Neutral"},
)
```

### JSON — Raw JSON Schema

Generate JSON matching a JSON Schema. Works with OpenAI and local models:

```go
schema := map[string]any{
    "type": "object",
    "properties": map[string]any{
        "name": map[string]any{"type": "string"},
        "age":  map[string]any{"type": "integer"},
    },
    "required": []string{"name", "age"},
}
jsonStr, _ := model.JSON("Generate a profile.", schema)
```

### Regex — Pattern Matching

Generate text matching a regex. Works with local models:

```go
ip, _ := model.Regex(
    "What is the IP address of localhost?",
    `\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`,
)
```

### Text — Unconstrained

```go
text, _ := model.Text("Tell me a joke.")
```

## API Reference

| Method | Description | Providers |
|--------|-------------|-----------|
| `PydanticJSON(prompt, name, fields)` | Structured JSON via Pydantic model | All |
| `PydanticJSONTo(prompt, name, fields, &dest)` | PydanticJSON + unmarshal to Go struct | All |
| `Choice(prompt, choices)` | Force one of the given strings | OpenAI, Local |
| `JSON(prompt, schema)` | JSON matching a JSON Schema | OpenAI, Local |
| `JSONTo(prompt, schema, &dest)` | JSON + unmarshal to Go struct | OpenAI, Local |
| `Regex(prompt, pattern)` | Text matching a regex pattern | Local |
| `Text(prompt)` | Unconstrained text generation | All |

> **Note:** Provider support varies. `PydanticJSON` is the most portable method and works across all providers including Ollama and Anthropic.
