# peftgo

Go bindings for [PEFT](https://github.com/huggingface/peft) (Parameter-Efficient Fine-Tuning) — powered by [pyffi](https://github.com/i2y/pyffi).

Fine-tune large language models from Go using LoRA and other adapter methods. Only a small fraction of parameters are trained, enabling efficient customization on consumer hardware.

## Install

```bash
go get github.com/i2y/pyffi/peftgo
```

Python 3.12+ and PyTorch are required at runtime. All packages are auto-installed via uv on first use.

## Quick Start

```go
// Load a base model
model, _ := peftgo.NewModel("meta-llama/Llama-3-8B",
    peftgo.WithDevice("auto"),
    peftgo.WithDtype("float16"),
)
defer model.Close()

// Apply LoRA adapter
model.ApplyLoRA(peftgo.LoRAConfig{
    Rank:  16,
    Alpha: 32,
})

// Check trainable parameters
trainable, total, pct := model.TrainableParams()
fmt.Printf("Trainable: %d / %d (%.2f%%)\n", trainable, total, pct)

// Save adapter (only the small adapter weights, not the full model)
model.SaveAdapter("./my-adapter")
```

## Load a Saved Adapter

```go
model, _ := peftgo.LoadAdapter("meta-llama/Llama-3-8B", "./my-adapter",
    peftgo.WithDevice("auto"),
)
defer model.Close()

text, _ := model.Generate("The meaning of life is", 50)
fmt.Println(text)
```

## LoRA Configuration

```go
peftgo.LoRAConfig{
    Rank:          16,                           // LoRA rank (default: 8)
    Alpha:         32,                           // scaling factor (default: 16)
    Dropout:       0.05,                         // dropout rate
    TargetModules: []string{"q_proj", "v_proj"}, // specific layers (empty = auto)
    TaskType:      "CAUSAL_LM",                  // CAUSAL_LM, SEQ_2_SEQ_LM, TOKEN_CLS, SEQ_CLS
}
```

## Why Wrap PEFT?

LoRA fine-tuning requires PyTorch for gradient computation and model weight manipulation. This cannot be reimplemented in Go. With peftgo, you can:

- Fine-tune models from Go applications
- Save tiny adapter files (MBs instead of GBs)
- Load adapters for inference without the full model weights
- Integrate fine-tuning into Go pipelines

## API Reference

| Method | Description |
|--------|-------------|
| `NewModel(id, opts...)` | Load a pretrained base model |
| `LoadAdapter(baseID, path, opts...)` | Load a model with a saved adapter |
| `model.ApplyLoRA(config)` | Apply LoRA adapter to the model |
| `model.TrainableParams()` | Get (trainable, total, percentage) |
| `model.SaveAdapter(path)` | Save adapter weights |
| `model.Generate(prompt, maxTokens)` | Generate text |
| `model.Close()` | Release resources |
