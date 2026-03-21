# polarsgo

Go bindings for [Polars](https://pola.rs) — powered by [pyffi](https://github.com/i2y/pyffi).

Polars is a blazingly fast DataFrame library built on Apache Arrow. This package lets you use it directly from Go — read CSV/JSON/Parquet, filter, sort, group, aggregate, join, and more. Includes full LazyFrame support for optimized query execution and SQL interface.

## Install

```bash
go get github.com/i2y/pyffi/polarsgo
```

Python 3.12+ is required at runtime. The `polars` package is auto-installed via uv on first use.

## Quick Start

```go
pl, _ := polarsgo.New()
defer pl.Close()

df, _ := pl.FromMaps([]map[string]any{
    {"name": "Alice", "age": 30, "dept": "Engineering"},
    {"name": "Bob", "age": 25, "dept": "Marketing"},
    {"name": "Charlie", "age": 35, "dept": "Engineering"},
    {"name": "Diana", "age": 28, "dept": "Marketing"},
})
defer df.Close()

fmt.Println(df)
// shape: (4, 3)
// ┌─────┬─────────────┬─────────┐
// │ age ┆ dept        ┆ name    │
// ╞═════╪═════════════╪═════════╡
// │ 30  ┆ Engineering ┆ Alice   │
// │ 25  ┆ Marketing   ┆ Bob     │
// │ 35  ┆ Engineering ┆ Charlie │
// │ 28  ┆ Marketing   ┆ Diana   │
// └─────┴─────────────┴─────────┘
```

## Read / Write Files

```go
df, _ := pl.ReadCSV("data.csv")
df, _ := pl.ReadJSON("data.json")
df, _ := pl.ReadParquet("data.parquet")

df.WriteCSV("output.csv")
df.WriteParquet("output.parquet")
df.WriteJSON("output.json")
```

## Filter, Sort, Select

```go
filtered, _ := df.Filter("col('age') > 28")
sorted, _ := df.Sort("age", true)  // descending
selected, _ := df.SelectCols("name", "dept")
dropped, _ := df.Drop("dept")
```

## Transform Columns

```go
transformed, _ := df.WithColumns("col('age') + 1", "col('name').str.to_uppercase()")
renamed, _ := df.Rename("name", "full_name", "age", "years")
```

## GroupBy & Aggregate

```go
grouped, _ := df.GroupBy("dept").Agg("col('age').mean()")
// ┌─────────────┬──────┐
// │ dept        ┆ age  │
// ╞═════════════╪══════╡
// │ Engineering ┆ 32.5 │
// │ Marketing   ┆ 26.5 │
// └─────────────┴──────┘
```

## Join

```go
salaries, _ := pl.FromMaps([]map[string]any{
    {"name": "Alice", "salary": 100000},
    {"name": "Bob", "salary": 80000},
})
joined, _ := df.Join(salaries, "name", "inner")
```

## LazyFrame

Polars' killer feature — build a query plan and let the optimizer execute it efficiently:

```go
lf, _ := df.Lazy()
result, _ := lf.
    Filter("col('age') > 26").
    WithColumns("col('age') * 2").
    Sort("age", true).
    Collect()
defer result.Close()
fmt.Println(result)
```

View the optimized query plan:

```go
lf, _ := df.Lazy()
fmt.Println(lf.Filter("col('age') > 26").Sort("age", true).Explain())
// SORT BY [descending: [true]] [col("age")]
//   FILTER [(col("age")) > (26)]
//   FROM
//     DF ["age", "dept", "name"]; PROJECT */3 COLUMNS
```

## Concat

```go
combined, _ := pl.Concat(df1, df2, df3)
```

## SQL

```go
result, _ := pl.SQL(
    "SELECT dept, AVG(age) as avg_age FROM employees GROUP BY dept ORDER BY avg_age DESC",
    map[string]*polarsgo.DataFrame{"employees": df},
)
// ┌─────────────┬─────────┐
// │ dept        ┆ avg_age │
// ╞═════════════╪═════════╡
// │ Engineering ┆ 32.5    │
// │ Marketing   ┆ 26.5    │
// └─────────────┴─────────┘
```

## Null Handling

```go
cleaned, _ := df.DropNulls()
filled, _ := df.FillNull(0)
unique, _ := df.Unique("name")
```

## Convert to Go Types

```go
maps, _ := df.ToMaps()     // []map[string]any
jsonStr, _ := df.ToJSON()   // JSON string
```

## Inspect

```go
rows, cols := df.Shape()
names := df.Columns()
fmt.Println(df.Head(2))
fmt.Println(df.Tail(2))
fmt.Println(df.Describe())
```

## API Reference

### Client

| Method | Description |
|--------|-------------|
| `New()` | Create a Polars client |
| `ReadCSV(path)` | Read CSV into DataFrame |
| `ReadJSON(path)` | Read JSON into DataFrame |
| `ReadParquet(path)` | Read Parquet into DataFrame |
| `FromMaps(data)` | Create DataFrame from `[]map[string]any` |
| `Concat(dfs...)` | Concatenate DataFrames vertically |
| `SQL(query, tables)` | Execute SQL query |

### DataFrame

| Method | Description |
|--------|-------------|
| `Shape()` | Returns (rows, cols) |
| `Columns()` | Column names |
| `Head(n)` / `Tail(n)` | First/last n rows |
| `Describe()` | Summary statistics |
| `Filter(expr)` | Filter rows with Polars expression |
| `Select(exprs...)` | Select by expression |
| `SelectCols(cols...)` | Select columns by name |
| `Drop(cols...)` | Remove columns |
| `WithColumns(exprs...)` | Add/transform columns |
| `Sort(col, desc)` | Sort by column |
| `Rename(old, new, ...)` | Rename columns |
| `Join(other, on, how)` | Join DataFrames |
| `JoinOn(other, leftOn, rightOn, how)` | Join on different column names |
| `GroupBy(cols...).Agg(exprs...)` | Group and aggregate |
| `Unique(cols...)` | Deduplicate rows |
| `DropNulls(cols...)` | Remove null rows |
| `FillNull(value)` | Replace nulls |
| `Pivot(on, index, values)` | Pivot table |
| `Unpivot(on, index)` | Wide to long format |
| `Lazy()` | Convert to LazyFrame |
| `ToMaps()` | Convert to `[]map[string]any` |
| `ToJSON()` | Convert to JSON string |
| `WriteCSV(path)` | Write to CSV |
| `WriteParquet(path)` | Write to Parquet |
| `WriteJSON(path)` | Write to JSON |
| `Close()` | Release resources |

### LazyFrame

| Method | Description |
|--------|-------------|
| `Filter(expr)` | Filter rows |
| `Select(exprs...)` | Select by expression |
| `WithColumns(exprs...)` | Add/transform columns |
| `Sort(col, desc)` | Sort |
| `Rename(old, new, ...)` | Rename columns |
| `Join(other, on, how)` | Join LazyFrames |
| `GroupBy(cols...).Agg(exprs...)` | Group and aggregate |
| `Unique(cols...)` | Deduplicate |
| `DropNulls()` | Remove null rows |
| `Limit(n)` | Limit rows |
| `Collect()` | Execute and return DataFrame |
| `Explain()` | Show optimized query plan |
| `Close()` | Release resources |
