---
name: pyffi
description: "Guide for writing Go code that calls Python using the pyffi library (CPython bindings via purego, no Cgo). Use this skill when writing Go code that interacts with Python, importing Python modules from Go, using the casdk (Claude Agent SDK) wrapper, or working with pyffi APIs like Runtime, Object, Exec, Eval, Import, type conversions, async patterns, callbacks, or GIL management. Also use when the user mentions purego + Python, embedding Python in Go, Go-Python FFI, or uv dependency management for Python packages from Go."
license: MIT
compatibility: "Requires Python 3.12+ shared library at runtime. macOS and Linux supported."
metadata:
  author: Yasushi Itoh
  version: "1.0"
---

# pyffi Development Guide

pyffi is a pure Go library that calls CPython via purego — no Cgo required. All methods are goroutine-safe thanks to automatic GIL management.

For the full API index, read `llms.txt` at the project root. For detailed signatures, read `llms-full.txt`.

## Basic Usage

```go
rt, err := pyffi.New()
if err != nil { log.Fatal(err) }
defer rt.Close()

rt.Exec(`x = 1 + 2`)                    // execute statements
result, _ := rt.Eval("x * 10")          // evaluate expressions → *Object
defer result.Close()                      // ALWAYS close Objects
val, _ := result.Int64()                 // extract Go value
```

## Critical Rules

1. **Always `defer obj.Close()`** on every `*Object` returned by pyffi. Forgetting this leaks Python references.

2. **Auto-GIL** — all public methods automatically acquire and release the Python GIL. Users never need `WithGIL` for correctness. Any goroutine can safely call pyffi methods directly. `WithGIL` is only useful for batching multiple operations to reduce per-call overhead.

3. **Type conversions are automatic** in both directions:
   - Go → Python: bool, int (all sizes), uint (all sizes including uint64), float, string, []byte, []any, map[string]any, nil
   - Python → Go: bool→bool, int→int64, float→float64, str→string, bytes/bytearray→[]byte, list/tuple/set→[]any, dict→map[string]any, None→nil

4. **Error handling**: Python exceptions become `*PythonError` with Type, Message, and Traceback (3.12+). Use `errors.As(err, &pyErr)` to extract details.

## Import and Call

```go
math, _ := rt.Import("math")
defer math.Close()
r, _ := math.Attr("sqrt").Call(16.0)  // chaining works — Attr returns nil on missing
defer r.Close()
f, _ := r.Float64()  // 4.0
```

## Collections

```go
list, _ := rt.NewList(1, "two", 3.0)
dict, _ := rt.NewDict("key", "value", "num", 42)  // alternating key-value pairs
tuple, _ := rt.NewTuple("a", "b")
set, _ := rt.NewSet(1, 2, 3)

item, _ := list.GetItem(0)       // list[0]
list.SetItem(0, 99)              // list[0] = 99
list.DelItem(0)                  // del list[0]
ok, _ := list.Contains(2)        // 2 in list
n, _ := list.Len()               // len(list)
repr, _ := list.Repr()           // repr(list)
```

## Iteration

```go
iter, _ := list.Iter()
defer iter.Close()
for {
    item, _ := iter.Next()
    if item == nil { break }  // nil means StopIteration
    defer item.Close()
}
```

## Keyword Arguments

```go
result, _ := fn.Call(arg1, arg2, pyffi.KW{"indent": 4, "sort_keys": true})
```

## Callbacks (Go → Python → Go)

```go
rt.RegisterFunc("add", func(a, b int) int { return a + b })

// With kwargs: last param must be map[string]any
rt.RegisterFunc("greet", func(name string, kw map[string]any) string {
    greeting := "hello"
    if g, ok := kw["greeting"]; ok {
        greeting = g.(string)
    }
    return greeting + " " + name
})

rt.Exec(`
import go_bridge
result = go_bridge.add(1, 2)
go_bridge.greet("Go", greeting="hi")
`)
```

## Async

```go
result, _ := rt.RunAsync("some_async_func()")      // synchronous wait
ch := rt.RunAsyncGo("fetch_data(url)")              // non-blocking
ar := <-ch  // pyffi.AsyncResult{Value, Err}

fn := mod.Attr("async_func")
result, _ := fn.CallAsync(arg1)                     // call async function
```

## Auto-Install Dependencies via uv

```go
rt, _ := pyffi.New(pyffi.Dependencies("numpy", "pandas"))  // auto-install
rt, _ := pyffi.New(pyffi.WithUVProject("/path/to/project")) // use pyproject.toml
```

## Context Managers

```go
rt.With(resource, func(value *pyffi.Object) error {
    // __enter__ called, __exit__ called automatically
    return nil
})
```

## Comparison

```go
eq, _ := a.Equals(b)                    // a == b
lt, _ := a.Compare(b, pyffi.PyLT)       // a < b
// Constants: PyLT, PyLE, PyEQ, PyNE, PyGT, PyGE
```

## casdk (Claude Agent SDK Wrapper)

For full casdk API, read `casdk/README.md`.

```go
import "github.com/i2y/pyffi/casdk"

// One-off query (needs ANTHROPIC_API_KEY)
client, _ := casdk.New()
defer client.Close()
for msg, err := range client.Query(ctx, "prompt", casdk.WithMaxTurns(1)) {
    if err != nil { break }
    fmt.Println(msg.Text())
}

// Interactive session (shares Runtime with client)
session, _ := client.Session(casdk.WithModel("sonnet"))
defer session.Close()
session.Query("Hello!")
for msg, _ := range session.ReceiveMessages() { ... }

// Session management (no API key needed)
sessions, _ := client.ListSessions(casdk.WithLimit(10))
msgs, _ := client.GetSessionMessages(sessions[0].SessionID)
```

## Code Generation (pyffi-gen)

Generate type-safe Go bindings from Python modules:

```bash
# Install
go install github.com/i2y/pyffi/cmd/pyffi-gen@latest

# Generate bindings
pyffi-gen --module numpy --out ./gen/numpypkg --dependencies numpy

# Preview without writing
pyffi-gen --module json --dry-run

# Config file
pyffi-gen --config pyffi-gen.yaml
```

The generated code provides a low-level typed `Module` wrapper. For production use, wrap it with an idiomatic public API:

```
yourpkg/
├── internal/sdk/   # Generated by pyffi-gen (DO NOT EDIT)
│   └── sdk.go
├── yourpkg.go      # Your idiomatic Go API wrapping internal/sdk
└── options.go
```

You don't need to wrap every generated function. You can also bypass the generated bindings and use `rt.Exec()`/`rt.Eval()` with Python code strings when that's simpler (e.g., complex objects, callbacks, many optional params). Mixing both approaches is fine. The `casdk` package demonstrates this — it uses generated bindings for some operations while using inline Python code generation for others.

## Common Mistakes

- Forgetting `defer obj.Close()` — every *Object must be closed
- Using `asyncio.run()` directly in Exec — use `rt.RunAsync()` instead
- Assuming `Attr()` returns error — it returns nil on missing (use `AttrErr()` for errors)
- Using `[]string` or `[]int` — pyffi only converts `[]any`
- Using `ReleaseGIL`/`WithGIL` for basic operations — auto-GIL handles this automatically

## Architecture

- **No Cgo**: purego loads libpython dynamically. `CGO_ENABLED=0` builds work.
- **Auto-GIL**: Every public method calls `autoGIL()` internally. PyGILState_Ensure nesting is safe.
- **Free-threaded Python**: 3.13t+ auto-detected, skips LockOSThread.
- **Platform**: macOS and Linux. Windows not yet supported.
