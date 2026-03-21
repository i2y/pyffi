# sbert

Go bindings for [sentence-transformers](https://www.sbert.net) — powered by [pyffi](https://github.com/i2y/pyffi).

Generate semantic embeddings and compute similarity from Go using 15,000+ pretrained models from Hugging Face. No Python server needed — the model runs in-process.

## Install

```bash
go get github.com/i2y/pyffi/sbert
```

Python 3.12+ is required at runtime. The `sentence-transformers` package is auto-installed via uv on first use.

## Quick Start

```go
model, _ := sbert.New("all-MiniLM-L6-v2")
defer model.Close()

embeddings, _ := model.Encode([]string{
    "The weather is lovely today.",
    "It's so sunny outside!",
    "He drove to the stadium.",
})

sim, _ := model.Similarity(embeddings, embeddings)
// sim[0][1] ≈ 0.67  (weather sentences are similar)
// sim[0][2] ≈ 0.10  (weather vs stadium are not)
```

## API

### New / NewWithRuntime

```go
// Auto-install sentence-transformers and load model
model, _ := sbert.New("all-MiniLM-L6-v2")
defer model.Close()

// Or use an existing pyffi Runtime
rt, _ := pyffi.New(pyffi.Dependencies("sentence-transformers"))
defer rt.Close()
model, _ := sbert.NewWithRuntime(rt, "all-MiniLM-L6-v2")
defer model.Close()
```

### Encode

Convert sentences to embedding vectors. Returns `[][]float32` where each row is one embedding.

```go
embeddings, _ := model.Encode([]string{"Hello world", "Go is great"})
fmt.Println(len(embeddings))    // 2
fmt.Println(len(embeddings[0])) // 384 (depends on model)
```

### Similarity

Compute pairwise similarity between two sets of embeddings.

```go
sim, _ := model.Similarity(embeddings, embeddings)
// sim[i][j] = similarity between embeddings[i] and embeddings[j]
```

## Models

Any model from the [Hugging Face sentence-transformers hub](https://huggingface.co/models?library=sentence-transformers) works:

| Model | Dimensions | Speed | Use Case |
|-------|-----------|-------|----------|
| `all-MiniLM-L6-v2` | 384 | Fast | General purpose |
| `all-mpnet-base-v2` | 768 | Medium | Higher quality |
| `paraphrase-multilingual-MiniLM-L12-v2` | 384 | Fast | Multilingual |
