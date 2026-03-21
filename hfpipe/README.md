# hfpipe

Go bindings for [Hugging Face Transformers](https://huggingface.co/docs/transformers) pipelines — powered by [pyffi](https://github.com/i2y/pyffi).

Run local ML inference from Go — text generation, classification, summarization, translation, and more — using any of the 300,000+ models on Hugging Face Hub. No API keys needed.

## Install

```bash
go get github.com/i2y/pyffi/hfpipe
```

Python 3.12+ is required at runtime. The `transformers` and `torch` packages are auto-installed via uv on first use.

## Why Wrap Transformers?

Transformers depends on PyTorch for model inference with GPU/MPS acceleration. This entire ML stack (tokenizers, model weights, CUDA kernels) cannot be reimplemented in Go.

## Quick Start

### Text Generation

```go
pipe, _ := hfpipe.New("text-generation", "openai-community/gpt2")
defer pipe.Close()

results, _ := pipe.RunKw("The meaning of life is", map[string]any{
    "max_new_tokens": 30,
})
fmt.Println(results[0]["generated_text"])
```

### Text Classification

```go
pipe, _ := hfpipe.New("text-classification", "distilbert/distilbert-base-uncased-finetuned-sst-2-english")
defer pipe.Close()

results, _ := pipe.Run("I love this movie!", "This was terrible.")
for _, r := range results {
    fmt.Printf("%s (%.2f%%)\n", r["label"], r["score"].(float64)*100)
}
// POSITIVE (99.99%)
// NEGATIVE (99.96%)
```

## Supported Tasks

| Task | Example Model |
|------|---------------|
| `text-generation` | `openai-community/gpt2`, `google/gemma-2-2b` |
| `text-classification` | `distilbert/distilbert-base-uncased-finetuned-sst-2-english` |
| `summarization` | `facebook/bart-large-cnn` |
| `translation` | `Helsinki-NLP/opus-mt-en-fr` |
| `fill-mask` | `google-bert/bert-base-uncased` |
| `question-answering` | `distilbert/distilbert-base-cased-distilled-squad` |
| `feature-extraction` | `sentence-transformers/all-MiniLM-L6-v2` |
| `zero-shot-classification` | `facebook/bart-large-mnli` |
| `automatic-speech-recognition` | `openai/whisper-large-v3` |
| `image-classification` | `google/vit-base-patch16-224` |

## Options

```go
hfpipe.WithDevice("cuda")      // GPU inference
hfpipe.WithDevice("mps")       // Apple Silicon
hfpipe.WithDevice("auto")      // auto-select best device
hfpipe.WithDtype("float16")    // half-precision (faster, less memory)
hfpipe.WithDtype("bfloat16")   // brain floating point
hfpipe.WithBatchSize(8)        // batch inference
```

## API

### New

```go
pipe, _ := hfpipe.New(task, model, opts...)
defer pipe.Close()
```

### Run

```go
// Single or multiple inputs
results, _ := pipe.Run("input text")
results, _ := pipe.Run("input 1", "input 2", "input 3")
```

### RunKw

Pass task-specific parameters:

```go
results, _ := pipe.RunKw("prompt", map[string]any{
    "max_new_tokens":      50,
    "num_return_sequences": 3,
    "temperature":         0.8,
})
```

Results are returned as `[]map[string]any`. The keys depend on the task (e.g. `generated_text`, `label`, `score`, `summary_text`).
