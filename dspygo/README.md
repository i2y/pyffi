# dspygo

Go bindings for [DSPy](https://dspy.ai) — powered by [pyffi](https://github.com/i2y/pyffi).

DSPy is a framework for *programming* — rather than prompting — language models. Instead of hand-crafting prompts, you define typed signatures and let DSPy's optimizers automatically generate and improve prompts using training data.

## Install

```bash
go get github.com/i2y/pyffi/dspygo
```

Python 3.12+ is required at runtime. The `dspy` package is auto-installed via uv on first use.

## Why Wrap DSPy?

DSPy's core value — optimizers that automatically improve prompts via bootstrapping, grounded proposals, and discrete search — consists of sophisticated ML algorithms that cannot be practically reimplemented in Go. This wrapper gives Go programs direct access to:

- **Typed Signatures** with `Literal` constraints for structured, validated outputs
- **Module Pipelines** for composing multi-step AI workflows
- **Optimizers** (BootstrapRS, MIPROv2) that compile modules into optimized prompts using training data

## Quick Start

```go
client, _ := dspygo.New(dspygo.WithLM("openai/gpt-4o-mini"))
defer client.Close()

cot := client.ChainOfThought("question -> answer")
result, _ := cot.Call(dspygo.KV{"question": "What is the capital of France?"})
fmt.Println(result.Get("answer")) // Paris
```

## Typed Signatures

Define input/output contracts with type constraints. `Literal` restricts the output to a fixed set of values — the LM is forced to choose one.

```go
classify := client.PredictSig(dspygo.Signature{
    Doc: "Classify sentiment of a given sentence.",
    Inputs: []dspygo.Field{
        {Name: "sentence", Type: "str", Description: "The sentence to classify"},
    },
    Outputs: []dspygo.Field{
        {Name: "sentiment", Type: `Literal["positive", "negative", "neutral"]`},
        {Name: "confidence", Type: "float"},
    },
})

result, _ := classify.Call(dspygo.KV{
    "sentence": "This book was super fun to read, I loved it!",
})
fmt.Println(result.Get("sentiment"))  // positive
fmt.Println(result.Get("confidence")) // 0.9
```

## Module Pipelines

Compose multi-step workflows where each step's output feeds into the next.

```go
pipe := client.NewPipeline(
    dspygo.PipelineStep{Name: "summarize", Kind: "ChainOfThought", Signature: "text -> summary"},
    dspygo.PipelineStep{Name: "translate", Kind: "Predict", Signature: "summary -> japanese"},
)

result, _ := pipe.Run(dspygo.KV{
    "text": "Go is a statically typed, compiled language designed at Google...",
})
fmt.Println(result.Get("japanese"))
```

## Optimizers

The killer feature of DSPy. Optimizers take a module and training examples, then automatically find better prompts and few-shot examples.

```go
// Define a module
qa := client.ChainOfThought("question -> answer")

// Provide training examples
trainset := []dspygo.Example{
    {Inputs: dspygo.KV{"question": "What is 2+2?"}, Outputs: dspygo.KV{"answer": "4"}},
    {Inputs: dspygo.KV{"question": "Capital of Japan?"}, Outputs: dspygo.KV{"answer": "Tokyo"}},
    // ... more examples
}

// Optimize — DSPy bootstraps better prompts automatically
optimized, _ := client.Optimize(qa, trainset, dspygo.OptimizeConfig{
    Optimizer: "BootstrapRS",
    Metric:    "dspy.evaluate.answer_exact_match",
})

// Use the optimized module (better prompts, few-shot examples selected)
result, _ := optimized.Call(dspygo.KV{"question": "Capital of France?"})
fmt.Println(result.Get("answer")) // Paris
```

### Available Optimizers

| Optimizer | Description | Use Case |
|-----------|-------------|----------|
| `BootstrapRS` | Synthesizes few-shot examples via random search | Quick improvement, small datasets |
| `MIPROv2` | Proposes and explores better instructions via grounded search | Higher quality, larger datasets |

## API Reference

### Client

```go
// OpenAI
client, _ := dspygo.New(dspygo.WithLM("openai/gpt-4o-mini"))

// Anthropic
client, _ := dspygo.New(
    dspygo.WithLM("anthropic/claude-3-haiku-20240307"),
    dspygo.WithAPIKey("sk-ant-..."),
)
```

DSPy uses [LiteLLM](https://docs.litellm.ai/) under the hood, supporting 100+ LLM providers.

### Modules

| Method | Description |
|--------|-------------|
| `client.Predict(sig)` | Basic prediction from string signature |
| `client.ChainOfThought(sig)` | Prediction with reasoning steps |
| `client.PredictSig(Signature{...})` | Prediction from typed signature |
| `client.ChainOfThoughtSig(Signature{...})` | ChainOfThought from typed signature |

### Signatures

String format — concise:
```
"question -> answer"                    // basic
"question -> answer: float"             // typed output
"context, question -> response"         // multiple inputs
```

Typed format — full control:
```go
dspygo.Signature{
    Doc:     "Task description used in the prompt",
    Inputs:  []dspygo.Field{{Name: "input", Type: "str"}},
    Outputs: []dspygo.Field{{Name: "output", Type: `Literal["a", "b", "c"]`}},
}
```

### Prediction

```go
result.Get("answer")    // get a specific field
result.Fields()         // all fields as map[string]string
```
