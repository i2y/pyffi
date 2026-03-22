# datasetsgo

Go bindings for [Hugging Face Datasets](https://github.com/huggingface/datasets) — powered by [pyffi](https://github.com/i2y/pyffi).

Access 200,000+ datasets on the Hugging Face Hub from Go. Load, filter, map, and iterate over datasets with automatic caching and streaming support.

## Install

```bash
go get github.com/i2y/pyffi/datasetsgo
```

Python 3.12+ is required at runtime. The `datasets` package is auto-installed via uv on first use.

## Quick Start

```go
client, _ := datasetsgo.New()
defer client.Close()

ds, _ := client.Load("imdb")
defer ds.Close()

train := ds.Split("train")
fmt.Println("Rows:", train.Len())
fmt.Println("Columns:", train.Columns())

row, _ := train.Row(0)
fmt.Println(row["text"])
fmt.Println(row["label"])
```

## Load Options

```go
// Specific split
ds, _ := client.Load("imdb", datasetsgo.WithSplit("train"))

// Streaming (no full download)
ds, _ := client.Load("imdb", datasetsgo.WithStreaming())

// Dataset subset/configuration
ds, _ := client.Load("glue", datasetsgo.WithSubset("mrpc"))
```

## Filter & Map

```go
// Filter rows
long, _ := train.Filter("lambda x: len(x['text']) > 500")

// Transform rows
mapped, _ := train.Map("lambda x: {'length': len(x['text'])}")
```

## Other Operations

```go
shuffled, _ := train.Shuffle(42)
sorted, _ := train.Sort("label")
subset, _ := train.Select([]int{0, 1, 2, 3, 4})
```

## Convert to Go Types

```go
rows, _ := train.ToMaps(10)   // first 10 rows as []map[string]any
jsonStr, _ := train.ToJSON()   // JSON string (up to 1000 rows)
```

## API Reference

### Client

| Method | Description |
|--------|-------------|
| `New()` | Create client |
| `Load(name, opts...)` | Load dataset from Hub |

### DatasetDict

| Method | Description |
|--------|-------------|
| `Splits()` | Available split names |
| `Split(name)` | Get a specific split |

### Dataset

| Method | Description |
|--------|-------------|
| `Len()` | Number of rows |
| `Columns()` | Column names |
| `Row(index)` | Get a single row |
| `Select(indices)` | Select rows by index |
| `Filter(lambda)` | Filter with Python lambda |
| `Map(lambda)` | Transform with Python lambda |
| `Shuffle(seed)` | Shuffle rows |
| `Sort(column)` | Sort by column |
| `ToMaps(n)` | Convert to `[]map[string]any` |
| `ToJSON()` | Convert to JSON string |
