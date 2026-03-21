# diffusersgo

Go bindings for [Hugging Face Diffusers](https://github.com/huggingface/diffusers) — powered by [pyffi](https://github.com/i2y/pyffi).

Generate images from text using Stable Diffusion and other diffusion models, directly from Go. No Python server needed — the model runs in-process.

## Install

```bash
go get github.com/i2y/pyffi/diffusersgo
```

Python 3.12+ and PyTorch are required at runtime. All Python packages are auto-installed via uv on first use.

## Quick Start

```go
pipe, _ := diffusersgo.New("stable-diffusion-v1-5/stable-diffusion-v1-5",
    diffusersgo.WithDevice("mps"),      // or "cuda" for NVIDIA GPU
    diffusersgo.WithDtype("float16"),
)
defer pipe.Close()

img, _ := pipe.TextToImage("A cat in an astronaut suit, oil painting")
img.Save("cat.png")
```

## Generation Options

```go
img, _ := pipe.TextToImage("A sunset over mountains",
    diffusersgo.WithSteps(50),               // inference steps
    diffusersgo.WithGuidanceScale(7.5),      // CFG scale
    diffusersgo.WithWidth(768),              // output width
    diffusersgo.WithHeight(512),             // output height
    diffusersgo.WithSeed(42),                // reproducibility
    diffusersgo.WithNegativePrompt("blurry"), // negative prompt
)
```

## Image Output

```go
img.Save("output.png")       // save to file
img.Save("output.jpg")       // format from extension

bytes, _ := img.Bytes()       // PNG bytes for HTTP response, etc.
```

## Models

Any model from the [Diffusers Hub](https://huggingface.co/models?library=diffusers) works:

| Model | Description |
|-------|-------------|
| `stable-diffusion-v1-5/stable-diffusion-v1-5` | Classic SD 1.5 |
| `stabilityai/stable-diffusion-xl-base-1.0` | SDXL |
| `black-forest-labs/FLUX.1-schnell` | FLUX.1 (fast) |
| `stabilityai/stable-diffusion-3-medium` | SD 3 |

## Device Support

```go
diffusersgo.WithDevice("cuda")   // NVIDIA GPU
diffusersgo.WithDevice("mps")    // Apple Silicon
diffusersgo.WithDevice("cpu")    // CPU (slow)
```

## API Reference

| Method | Description |
|--------|-------------|
| `New(model, opts...)` | Load a diffusion pipeline |
| `pipe.TextToImage(prompt, opts...)` | Generate image from text |
| `img.Save(path)` | Save image to file |
| `img.Bytes()` | Get image as PNG bytes |
| `pipe.Close()` / `img.Close()` | Release resources |
