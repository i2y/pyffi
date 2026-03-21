// Package diffusersgo provides Go bindings for [Hugging Face Diffusers],
// enabling image generation from Go using Stable Diffusion and other
// diffusion models.
//
// The Python packages are auto-installed via uv on first use.
//
//	pipe, _ := diffusersgo.New("stable-diffusion-v1-5/stable-diffusion-v1-5")
//	defer pipe.Close()
//
//	img, _ := pipe.TextToImage("A cat in space, oil painting")
//	img.Save("cat.png")
//
// [Hugging Face Diffusers]: https://github.com/huggingface/diffusers
package diffusersgo

import (
	"fmt"
	"strings"

	"github.com/i2y/pyffi"
)

// Pipeline wraps a Diffusers DiffusionPipeline.
type Pipeline struct {
	rt   *pyffi.Runtime
	pipe *pyffi.Object
}

// PipelineOption configures a Pipeline.
type PipelineOption func(*pipeConfig)

type pipeConfig struct {
	device string // "cuda", "mps", "cpu"
	dtype  string // "float16", "bfloat16", ""
}

// WithDevice sets the device for inference ("cuda", "mps", "cpu").
func WithDevice(device string) PipelineOption {
	return func(c *pipeConfig) { c.device = device }
}

// WithDtype sets the model precision ("float16", "bfloat16").
func WithDtype(dtype string) PipelineOption {
	return func(c *pipeConfig) { c.dtype = dtype }
}

// New loads a diffusion pipeline from a pretrained model.
func New(model string, opts ...PipelineOption) (*Pipeline, error) {
	var cfg pipeConfig
	for _, o := range opts {
		o(&cfg)
	}

	rt, err := pyffi.New(pyffi.Dependencies("diffusers[torch]", "transformers", "accelerate"))
	if err != nil {
		return nil, fmt.Errorf("diffusersgo: %w", err)
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("%q", model))
	if cfg.dtype == "float16" {
		parts = append(parts, "torch_dtype=torch.float16")
	} else if cfg.dtype == "bfloat16" {
		parts = append(parts, "torch_dtype=torch.bfloat16")
	}

	code := fmt.Sprintf(`
import torch
from diffusers import DiffusionPipeline
_dfgo_pipe = DiffusionPipeline.from_pretrained(%s)
`, strings.Join(parts, ", "))

	if cfg.device != "" {
		code += fmt.Sprintf("_dfgo_pipe = _dfgo_pipe.to(%q)\n", cfg.device)
	}

	if err := rt.Exec(code); err != nil {
		rt.Close()
		return nil, fmt.Errorf("diffusersgo: load model: %w", err)
	}

	mainMod, _ := rt.Import("__main__")
	defer mainMod.Close()
	pipe := mainMod.Attr("_dfgo_pipe")
	if pipe == nil {
		rt.Close()
		return nil, fmt.Errorf("diffusersgo: pipeline not found")
	}

	return &Pipeline{rt: rt, pipe: pipe}, nil
}

// Runtime returns the underlying pyffi Runtime.
func (p *Pipeline) Runtime() *pyffi.Runtime { return p.rt }

// Close releases all resources.
func (p *Pipeline) Close() error {
	p.pipe.Close()
	return p.rt.Close()
}

// GenerateOption configures image generation.
type GenerateOption func(*genConfig)

type genConfig struct {
	steps         int
	guidanceScale float64
	width         int
	height        int
	seed          int64
	hasSeed       bool
	negPrompt     string
}

// WithSteps sets the number of inference steps (default: model-dependent, typically 50).
func WithSteps(n int) GenerateOption {
	return func(c *genConfig) { c.steps = n }
}

// WithGuidanceScale sets the classifier-free guidance scale (default: 7.5).
func WithGuidanceScale(scale float64) GenerateOption {
	return func(c *genConfig) { c.guidanceScale = scale }
}

// WithWidth sets the output image width.
func WithWidth(w int) GenerateOption {
	return func(c *genConfig) { c.width = w }
}

// WithHeight sets the output image height.
func WithHeight(h int) GenerateOption {
	return func(c *genConfig) { c.height = h }
}

// WithSeed sets the random seed for reproducibility.
func WithSeed(seed int64) GenerateOption {
	return func(c *genConfig) { c.seed = seed; c.hasSeed = true }
}

// WithNegativePrompt sets the negative prompt.
func WithNegativePrompt(prompt string) GenerateOption {
	return func(c *genConfig) { c.negPrompt = prompt }
}

// TextToImage generates an image from a text prompt.
func (p *Pipeline) TextToImage(prompt string, opts ...GenerateOption) (*Image, error) {
	var cfg genConfig
	for _, o := range opts {
		o(&cfg)
	}

	mainMod, _ := p.rt.Import("__main__")
	defer mainMod.Close()

	promptObj := p.rt.FromString(prompt)
	defer promptObj.Close()
	mainMod.SetAttr("_dfgo_prompt", promptObj)

	var kwParts []string
	if cfg.steps > 0 {
		kwParts = append(kwParts, fmt.Sprintf("num_inference_steps=%d", cfg.steps))
	}
	if cfg.guidanceScale > 0 {
		kwParts = append(kwParts, fmt.Sprintf("guidance_scale=%f", cfg.guidanceScale))
	}
	if cfg.width > 0 {
		kwParts = append(kwParts, fmt.Sprintf("width=%d", cfg.width))
	}
	if cfg.height > 0 {
		kwParts = append(kwParts, fmt.Sprintf("height=%d", cfg.height))
	}
	if cfg.negPrompt != "" {
		kwParts = append(kwParts, fmt.Sprintf("negative_prompt=%q", cfg.negPrompt))
	}
	if cfg.hasSeed {
		kwParts = append(kwParts, fmt.Sprintf("generator=torch.Generator().manual_seed(%d)", cfg.seed))
	}

	kwargs := ""
	if len(kwParts) > 0 {
		kwargs = ", " + strings.Join(kwParts, ", ")
	}

	code := fmt.Sprintf("_dfgo_result = _dfgo_pipe(_dfgo_prompt%s).images[0]\n", kwargs)

	if err := p.rt.Exec(code); err != nil {
		return nil, fmt.Errorf("diffusersgo: generate: %w", err)
	}

	imgObj := mainMod.Attr("_dfgo_result")
	if imgObj == nil {
		return nil, fmt.Errorf("diffusersgo: no image result")
	}

	return &Image{rt: p.rt, img: imgObj}, nil
}

// Image wraps a PIL Image.
type Image struct {
	rt  *pyffi.Runtime
	img *pyffi.Object
}

// Save saves the image to a file (PNG, JPEG, etc. based on extension).
func (img *Image) Save(path string) error {
	fn := img.img.Attr("save")
	if fn == nil {
		return fmt.Errorf("diffusersgo: save not found")
	}
	defer fn.Close()
	result, err := fn.Call(path)
	if err != nil {
		return fmt.Errorf("diffusersgo: save: %w", err)
	}
	if result != nil {
		result.Close()
	}
	return nil
}

// Bytes returns the image as PNG bytes.
func (img *Image) Bytes() ([]byte, error) {
	code := `
import io as _io
_dfgo_buf = _io.BytesIO()
_dfgo_img.save(_dfgo_buf, format="PNG")
_dfgo_img_bytes = _dfgo_buf.getvalue()
`
	mainMod, _ := img.rt.Import("__main__")
	defer mainMod.Close()
	mainMod.SetAttr("_dfgo_img", img.img)
	if err := img.rt.Exec(code); err != nil {
		return nil, fmt.Errorf("diffusersgo: bytes: %w", err)
	}
	bytesObj := mainMod.Attr("_dfgo_img_bytes")
	if bytesObj == nil {
		return nil, fmt.Errorf("diffusersgo: no bytes")
	}
	defer bytesObj.Close()
	return bytesObj.GoBytes()
}

// Close releases the image.
func (img *Image) Close() error {
	return img.img.Close()
}
