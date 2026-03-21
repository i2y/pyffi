# pyffi

Go bindings for CPython via [purego](https://github.com/ebitengine/purego) — **no Cgo required**.

```go
rt, _ := pyffi.New()
defer rt.Close()

rt.Exec(`x = 1 + 2`)
result, _ := rt.Eval("x * 10")
defer result.Close()
fmt.Println(result.Int64()) // 30
```

## Features

- **No Cgo** — pure Go, uses purego for FFI. `CGO_ENABLED=0` builds work
- **Auto-GIL** — all methods are goroutine-safe out of the box
- **Full type conversion** — bool, int, float, string, bytes, list, tuple, dict, set
- **Async support** — `RunAsync`, `CallAsync`, `EventLoop` for asyncio integration
- **Callbacks** — register Go functions callable from Python (with kwargs)
- **Free-threaded Python** — automatic detection of Python 3.13t+ builds
- **uv integration** — auto-detect, auto-install dependencies, project venv support
- **Code generation** — `pyffi-gen` generates type-safe Go bindings from Python modules
- **Platform support** — macOS and Linux (Windows is not yet supported)

## Install

```bash
go get github.com/i2y/pyffi
```

Python 3.12+ is needed at runtime. pyffi auto-detects system Python, Homebrew, and [uv](https://github.com/astral-sh/uv)-managed installations. If Python is not installed:

```bash
# Recommended: install uv, then let pyffi auto-detect
curl -LsSf https://astral.sh/uv/install.sh | sh
uv python install 3.14
```

## Quick Start

### Execute and Evaluate

```go
rt, _ := pyffi.New()
defer rt.Close()

// Execute statements
rt.Exec(`
def greet(name):
    return f"Hello, {name}!"
`)

// Evaluate expressions
result, _ := rt.Eval(`greet("World")`)
defer result.Close()
s, _ := result.GoString() // "Hello, World!"
```

### Resource Management (Close)

pyffi wraps Python objects as `*Object` values that hold a reference to the underlying CPython object. These must be released with `Close()` to avoid memory leaks:

| Type | Close required? | Why |
|------|----------------|-----|
| `*Runtime` | Yes (`defer rt.Close()`) | Calls `Py_Finalize` to shut down the interpreter |
| `*Object` | Yes (`defer obj.Close()`) | Calls `Py_DecRef` to release the Python reference |

Every method that returns `*Object` — `Eval`, `Import`, `Attr`, `Call`, `GetItem`, `Iter().Next()`, etc. — requires the caller to close the result. A GC finalizer exists as a safety net, but explicit `Close()` is strongly recommended.

Primitive extraction methods (`Int64`, `Float64`, `GoString`, `Bool`, `GoSlice`, `GoMap`, `GoValue`) return plain Go values that do **not** need closing.

### Import Modules

```go
math, _ := rt.Import("math")
defer math.Close()

pi := math.Attr("pi")
defer pi.Close()
f, _ := pi.Float64() // 3.141592653589793
```

### Collections

```go
// Create
list, _ := rt.NewList(1, "two", 3.0)
dict, _ := rt.NewDict("name", "Go", "year", 2009)
tuple, _ := rt.NewTuple("a", "b", "c")
set, _ := rt.NewSet(1, 2, 3)

// Access
item, _ := list.GetItem(0)      // list[0]
list.SetItem(0, 99)             // list[0] = 99
list.DelItem(0)                 // del list[0]
ok, _ := list.Contains(2)       // 2 in list
n, _ := list.Len()              // len(list)
r, _ := list.Repr()             // repr(list)

// Iterate
iter, _ := list.Iter()
defer iter.Close()
for {
    item, _ := iter.Next()
    if item == nil { break }
    defer item.Close()
    // ...
}

// Compare
eq, _ := a.Equals(b)            // a == b
lt, _ := a.Compare(b, pyffi.PyLT)  // a < b
```

### Type Conversions

| Go → Python | Python → Go |
|---|---|
| `bool` → `bool` | `bool` → `bool` |
| `int`, `int8`–`int64` → `int` | `int` → `int64` |
| `uint`, `uint8`–`uint64` → `int` | `float` → `float64` |
| `float32`, `float64` → `float` | `str` → `string` |
| `string` → `str` | `bytes`, `bytearray` → `[]byte` |
| `[]byte` → `bytes` | `list`, `tuple`, `set` → `[]any` |
| `[]any` → `list` | `dict` → `map[string]any` |
| `map[string]any` → `dict` | `None` → `nil` |
| `nil` → `None` | |

Other Python types (functions, classes, instances, modules, etc.) are returned as `*Object` and can be manipulated via `Attr()`, `Call()`, `GetItem()`, etc:

```go
// Classes: Call() instantiates
cls := mod.Attr("MyClass")
defer cls.Close()
instance, _ := cls.Call("arg1", 42)  // MyClass("arg1", 42)
defer instance.Close()

// Instance methods and attributes
name, _ := instance.Attr("name").GoString()
result, _ := instance.Attr("method").Call()
defer result.Close()

// Functions: first-class objects
fn := mod.Attr("some_function")
defer fn.Close()
result, _ := fn.Call(args...)
```

### Goroutine Safety

All methods automatically acquire the GIL. No manual management needed:

```go
var wg sync.WaitGroup
for i := range 10 {
    wg.Add(1)
    go func(n int) {
        defer wg.Done()
        obj := rt.FromInt64(int64(n))
        defer obj.Close()
        // Safe from any goroutine
    }(i)
}
wg.Wait()
```

For batching (reduces GIL overhead):

```go
rt.WithGIL(func() error {
    rt.Exec("a = 1")
    rt.Exec("b = 2")
    rt.Exec("c = a + b")
    return nil
})
```

### Async Python

```go
// Synchronous
result, _ := rt.RunAsync("fetch_data('https://example.com')")

// Non-blocking (background goroutine)
ch := rt.RunAsyncGo("fetch_data('https://example.com')")
ar := <-ch // pyffi.AsyncResult{Value, Err}

// Call async functions directly
fn := mod.Attr("async_func")
result, _ := fn.CallAsync(arg1, arg2)
```

### Callbacks

```go
rt.RegisterFunc("add", func(a, b int) int {
    return a + b
})

// With keyword arguments
rt.RegisterFunc("greet", func(name string, kw map[string]any) string {
    greeting := "hello"
    if g, ok := kw["greeting"]; ok {
        greeting = g.(string)
    }
    return greeting + " " + name
})

rt.Exec(`
import go_bridge
print(go_bridge.add(1, 2))               # 3
print(go_bridge.greet("Go", greeting="hi"))  # hi Go
`)
```

### Context Managers

```go
rt.With(resource, func(value *pyffi.Object) error {
    // __enter__ called, value is the result
    // __exit__ called automatically (even on error)
    return nil
})
```

## Python & Dependency Management with uv

pyffi integrates with [uv](https://github.com/astral-sh/uv) for Python discovery and dependency management. uv is a fast Python package manager written in Rust.

### Auto-Detect uv-Managed Python

```go
// Prefer uv-managed Python installations
rt, _ := pyffi.New(pyffi.WithUV())
```

### Inline Dependencies

Automatically creates a hash-based cached venv in `~/.cache/pyffi/venvs/` and installs packages:

```go
rt, _ := pyffi.New(pyffi.Dependencies("numpy", "pandas", "requests"))
defer rt.Close()

rt.Exec(`
import numpy as np
import pandas as pd
print(np.array([1, 2, 3]))
`)
```

The venv is cached by a hash of the dependency list — subsequent runs skip installation.

### Project venv (pyproject.toml)

For projects with a `pyproject.toml`, use `WithUVProject` to run `uv sync` and use the project's venv:

```go
rt, _ := pyffi.New(pyffi.WithUVProject("/path/to/python-project"))
defer rt.Close()

// All project dependencies are available
rt.Exec(`from mypackage import something`)
```

### Explicit Library Path

Skip auto-detection entirely by pointing directly to the Python shared library:

```go
rt, _ := pyffi.New(pyffi.WithLibraryPath("/usr/lib/libpython3.14.so"))
```

This is useful in containers where the Python location is known at build time.

## Code Generation

`pyffi-gen` generates type-safe Go bindings from Python modules by introspecting them at build time:

```bash
# Install
go install github.com/i2y/pyffi/cmd/pyffi-gen@latest

# Generate bindings for a Python module
pyffi-gen --module numpy --out ./gen/numpypkg --dependencies numpy

# Preview without writing files
pyffi-gen --module json --dry-run

# Use a config file
pyffi-gen --config pyffi-gen.yaml

# Initialize a new project with config scaffolding
pyffi-gen init --module numpy,pandas
```

The generated code provides a low-level typed `Module` wrapper. For production use, create a higher-level Go package that wraps the generated code with idiomatic APIs:

```
yourpkg/
├── internal/sdk/       # Generated by pyffi-gen (DO NOT EDIT)
│   └── sdk.go
├── yourpkg.go          # Your idiomatic Go API wrapping internal/sdk
├── options.go          # Option types
└── ...
```

You don't need to wrap every generated function — pick and choose what to expose. You can also bypass the generated bindings entirely and use `rt.Exec()` / `rt.Eval()` with Python code strings for cases where that's simpler (e.g., variadic expression arguments). Mixing both approaches in the same package is fine. The `polarsgo` package in this repository is a practical example — it uses the generated `internal/sdk/` for methods with fixed signatures (Head, Tail, Join, Sort, etc.) while using inline Python code for expression-based methods (Filter, WithColumns, GroupBy).

## Docker Deployment

pyffi works well in Docker with a multi-stage build. Since pyffi uses purego (no Cgo), the Go binary can be built with `CGO_ENABLED=0`:

```dockerfile
# Stage 1: Build Go binary (no Python needed)
FROM golang:1.26 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /app/myapp ./cmd/myapp

# Stage 2: Runtime with Python
FROM python:3.14-slim
RUN pip install uv
COPY --from=builder /app/myapp /usr/local/bin/myapp

# Option A: Use pyproject.toml
COPY pyproject.toml uv.lock ./
RUN uv sync
# In Go: pyffi.New(pyffi.WithUVProject("."))

# Option B: Or let pyffi.Dependencies() install at first run
# In Go: pyffi.New(pyffi.Dependencies("numpy", "pandas"))

CMD ["myapp"]
```

### Optimized: No uv at Runtime

For smaller images, install dependencies at build time and use `WithLibraryPath` to skip uv at runtime:

```dockerfile
FROM python:3.14-slim AS python-deps
RUN pip install uv
COPY pyproject.toml uv.lock ./
RUN uv sync

FROM python:3.14-slim
COPY --from=python-deps /.venv /.venv
COPY --from=builder /app/myapp /usr/local/bin/myapp
# No uv needed at runtime
# In Go: pyffi.New(pyffi.WithLibraryPath("/usr/local/lib/libpython3.14.so"))
# Then add /.venv/lib/python3.14/site-packages to sys.path via Exec
CMD ["myapp"]
```

## Wrapper Packages

pyffi-powered Go bindings for popular Python libraries. Each is an independent Go module — install only what you need.

### polarsgo — DataFrames

Fast DataFrame operations from Go using [Polars](https://pola.rs). Filter, sort, join, group, aggregate, LazyFrame optimization, and SQL queries.

```bash
go get github.com/i2y/pyffi/polarsgo
```

```go
pl, _ := polarsgo.New()
defer pl.Close()

df, _ := pl.ReadCSV("data.csv")
result, _ := df.Filter("col('age') > 30").Sort("age", true)
fmt.Println(result)

// SQL queries
sqlResult, _ := pl.SQL("SELECT dept, AVG(age) FROM t GROUP BY dept", map[string]*polarsgo.DataFrame{"t": df})
```

See the [polarsgo README](polarsgo/README.md) for the full API including LazyFrame, Join, GroupBy, and more.

### sbert — Sentence Embeddings

Generate semantic embeddings using 15,000+ [sentence-transformers](https://www.sbert.net) models.

```bash
go get github.com/i2y/pyffi/sbert
```

```go
model, _ := sbert.New("all-MiniLM-L6-v2")
defer model.Close()

embeddings, _ := model.Encode([]string{"Hello world", "Go is great"})
sim, _ := model.Similarity(embeddings, embeddings)
```

See the [sbert README](sbert/README.md).

### hfpipe — Hugging Face Pipelines

Local ML inference — text generation, classification, summarization, and more.

```bash
go get github.com/i2y/pyffi/hfpipe
```

```go
pipe, _ := hfpipe.New("text-classification", "distilbert/distilbert-base-uncased-finetuned-sst-2-english")
defer pipe.Close()

results, _ := pipe.Run("I love this movie!")  // [{label:POSITIVE score:0.9999}]
```

See the [hfpipe README](hfpipe/README.md).

### dspygo — DSPy

Program (not prompt) language models with typed signatures, pipelines, and automatic prompt optimization.

```bash
go get github.com/i2y/pyffi/dspygo
```

```go
client, _ := dspygo.New(dspygo.WithLM("openai/gpt-4o-mini"))
defer client.Close()

classify := client.PredictSig(dspygo.Signature{
    Doc: "Classify sentiment.",
    Inputs:  []dspygo.Field{{Name: "sentence", Type: "str"}},
    Outputs: []dspygo.Field{{Name: "sentiment", Type: `Literal["positive", "negative", "neutral"]`}},
})
result, _ := classify.Call(dspygo.KV{"sentence": "I love it!"})
```

See the [dspygo README](dspygo/README.md).

### outlines — Structured Generation

Constrained decoding — guarantee LLM output matches a JSON schema, regex, or choice set.

```bash
go get github.com/i2y/pyffi/outlines
```

```go
model, _ := outlines.NewOllama("llama3.2")
defer model.Close()

result, _ := model.PydanticJSON("Generate a user profile.", "Profile", map[string]string{"name": "str", "age": "int"})
```

See the [outlines README](outlines/README.md).

### casdk — Claude Agent SDK

Go wrapper for the [Claude Agent SDK](https://github.com/anthropics/claude-agent-sdk-python) with hooks, plugins, and in-process MCP tools.

```bash
go get github.com/i2y/pyffi/casdk
```

```go
client, _ := casdk.New()
defer client.Close()

for msg, err := range client.Query(ctx, "What is 2+2?", casdk.WithMaxTurns(1)) {
    if err != nil { log.Fatal(err) }
    fmt.Println(msg.Text())
}
```

See the [casdk README](casdk/README.md).

## For LLM Agents

This project includes AgentSkills for AI coding assistants:

- [`skills/pyffi/SKILL.md`](skills/pyffi/SKILL.md) — pyffi usage guide
- [`casdk/skills/casdk/SKILL.md`](casdk/skills/casdk/SKILL.md) — casdk usage guide
- [`llms.txt`](llms.txt) — LLM-friendly API index
- [`llms-full.txt`](llms-full.txt) — Full documentation for LLM context

## Build & Test

```bash
make ci          # fmt + vet + build + test
make test        # verbose tests
make bench       # benchmarks
make lint        # staticcheck (if installed)
```

## License

MIT
