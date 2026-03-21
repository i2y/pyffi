// Package polarsgo provides Go bindings for [Polars], a fast DataFrame library.
//
// The Python package is auto-installed via uv on first use.
//
//	pl, _ := polarsgo.New()
//	defer pl.Close()
//
//	df, _ := pl.ReadCSV("data.csv")
//	fmt.Println(df.Head(5))
//
// [Polars]: https://pola.rs
package polarsgo

import (
	"encoding/json"
	"fmt"

	"github.com/i2y/pyffi"
	polars "github.com/i2y/pyffi/polarsgo/internal/sdk"
)

// Client wraps a Polars runtime.
type Client struct {
	rt  *pyffi.Runtime
	mod *polars.Module
}

// New creates a new Polars client.
func New() (*Client, error) {
	rt, err := pyffi.New(pyffi.Dependencies("polars"))
	if err != nil {
		return nil, fmt.Errorf("polarsgo: %w", err)
	}
	mod, err := polars.New(rt)
	if err != nil {
		rt.Close()
		return nil, fmt.Errorf("polarsgo: import: %w", err)
	}
	return &Client{rt: rt, mod: mod}, nil
}

// Close releases all resources.
func (c *Client) Close() error {
	c.mod.Close()
	return c.rt.Close()
}

// Runtime returns the underlying pyffi Runtime.
func (c *Client) Runtime() *pyffi.Runtime { return c.rt }

// --- DataFrame ---

// DataFrame wraps a Polars DataFrame.
type DataFrame struct {
	inner *polars.DataFrame
	rt    *pyffi.Runtime
}

func wrapDF(rt *pyffi.Runtime, df *polars.DataFrame) *DataFrame {
	return &DataFrame{inner: df, rt: rt}
}

// Close releases the DataFrame.
func (df *DataFrame) Close() error { return df.inner.Close() }

// Object returns the underlying Python object.
func (df *DataFrame) Object() *pyffi.Object { return df.inner.Object() }

// String returns a string representation.
func (df *DataFrame) String() string { return df.inner.Object().String() }

// Shape returns (rows, cols).
func (df *DataFrame) Shape() (int, int) {
	obj := df.inner.Object().Attr("shape")
	if obj == nil {
		return 0, 0
	}
	defer obj.Close()
	r, err := obj.GetItem(0)
	if err != nil {
		return 0, 0
	}
	defer r.Close()
	c, err := obj.GetItem(1)
	if err != nil {
		return 0, 0
	}
	defer c.Close()
	rows, _ := r.Int64()
	cols, _ := c.Int64()
	return int(rows), int(cols)
}

// Columns returns the column names.
func (df *DataFrame) Columns() []string {
	obj := df.inner.Object().Attr("columns")
	if obj == nil {
		return nil
	}
	defer obj.Close()
	sl, _ := obj.GoSlice()
	var names []string
	for _, v := range sl {
		if s, ok := v.(string); ok {
			names = append(names, s)
		}
	}
	return names
}

// Head returns the first n rows (generated binding).
func (df *DataFrame) Head(n int) *DataFrame {
	result, err := df.inner.Head(pyffi.KW{"n": n})
	if err != nil {
		return df
	}
	return wrapDF(df.rt, result)
}

// Tail returns the last n rows (generated binding).
func (df *DataFrame) Tail(n int) *DataFrame {
	result, err := df.inner.Tail(pyffi.KW{"n": n})
	if err != nil {
		return df
	}
	return wrapDF(df.rt, result)
}

// Describe returns summary statistics (generated binding).
func (df *DataFrame) Describe() *DataFrame {
	result, err := df.inner.Describe()
	if err != nil {
		return df
	}
	return wrapDF(df.rt, result)
}

// Sort sorts by column (generated binding).
func (df *DataFrame) Sort(col string, descending bool) (*DataFrame, error) {
	result, err := df.inner.Sort(col, pyffi.KW{"descending": descending})
	if err != nil {
		return nil, fmt.Errorf("polarsgo: sort: %w", err)
	}
	return wrapDF(df.rt, result), nil
}

// Join joins two DataFrames (generated binding).
func (df *DataFrame) Join(other *DataFrame, on string, how string) (*DataFrame, error) {
	result, err := df.inner.Join(other.inner, pyffi.KW{"on": on, "how": how})
	if err != nil {
		return nil, fmt.Errorf("polarsgo: join: %w", err)
	}
	return wrapDF(df.rt, result), nil
}

// JoinOn joins with different column names (generated binding).
func (df *DataFrame) JoinOn(other *DataFrame, leftOn, rightOn string, how string) (*DataFrame, error) {
	result, err := df.inner.Join(other.inner, pyffi.KW{"left_on": leftOn, "right_on": rightOn, "how": how})
	if err != nil {
		return nil, fmt.Errorf("polarsgo: join_on: %w", err)
	}
	return wrapDF(df.rt, result), nil
}

// Rename renames columns (generated binding).
func (df *DataFrame) Rename(pairs ...string) (*DataFrame, error) {
	if len(pairs)%2 != 0 {
		return nil, fmt.Errorf("polarsgo: rename requires even number of arguments")
	}
	mapping := map[string]any{}
	for i := 0; i < len(pairs); i += 2 {
		mapping[pairs[i]] = pairs[i+1]
	}
	result, err := df.inner.Rename(mapping)
	if err != nil {
		return nil, fmt.Errorf("polarsgo: rename: %w", err)
	}
	return wrapDF(df.rt, result), nil
}

// Unique returns unique rows (generated binding).
func (df *DataFrame) Unique(cols ...string) (*DataFrame, error) {
	if len(cols) > 0 {
		s := make([]any, len(cols))
		for i, c := range cols {
			s[i] = c
		}
		result, err := df.inner.Unique(pyffi.KW{"subset": s})
		if err != nil {
			return nil, fmt.Errorf("polarsgo: unique: %w", err)
		}
		return wrapDF(df.rt, result), nil
	}
	result, err := df.inner.Unique()
	if err != nil {
		return nil, fmt.Errorf("polarsgo: unique: %w", err)
	}
	return wrapDF(df.rt, result), nil
}

// DropNulls removes rows with null values (generated binding).
func (df *DataFrame) DropNulls(cols ...string) (*DataFrame, error) {
	if len(cols) > 0 {
		s := make([]any, len(cols))
		for i, c := range cols {
			s[i] = c
		}
		result, err := df.inner.DropNulls(pyffi.KW{"subset": s})
		if err != nil {
			return nil, fmt.Errorf("polarsgo: drop_nulls: %w", err)
		}
		return wrapDF(df.rt, result), nil
	}
	result, err := df.inner.DropNulls()
	if err != nil {
		return nil, fmt.Errorf("polarsgo: drop_nulls: %w", err)
	}
	return wrapDF(df.rt, result), nil
}

// FillNull replaces null values (generated binding).
func (df *DataFrame) FillNull(value any) (*DataFrame, error) {
	result, err := df.inner.FillNull(pyffi.KW{"value": value})
	if err != nil {
		return nil, fmt.Errorf("polarsgo: fill_null: %w", err)
	}
	return wrapDF(df.rt, result), nil
}

// Lazy converts to LazyFrame (generated binding).
func (df *DataFrame) Lazy() (*LazyFrame, error) {
	result, err := df.inner.Lazy()
	if err != nil {
		return nil, fmt.Errorf("polarsgo: lazy: %w", err)
	}
	return wrapLF(df.rt, result), nil
}

// ToMaps returns the DataFrame as a slice of maps.
func (df *DataFrame) ToMaps() ([]map[string]any, error) {
	result, err := df.inner.ToDicts()
	if err != nil {
		return nil, fmt.Errorf("polarsgo: to_dicts: %w", err)
	}
	switch v := result.(type) {
	case []any:
		var maps []map[string]any
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				maps = append(maps, m)
			}
		}
		return maps, nil
	default:
		return nil, fmt.Errorf("polarsgo: to_dicts: unexpected type %T", result)
	}
}

// WriteCSV writes to CSV file (generated binding).
func (df *DataFrame) WriteCSV(path string) error {
	_, err := df.inner.WriteCsv(pyffi.KW{"file": path})
	return err
}

// WriteParquet writes to Parquet file (generated binding).
func (df *DataFrame) WriteParquet(path string) error {
	return df.inner.WriteParquet(path)
}

// WriteJSON writes to JSON file (generated binding).
func (df *DataFrame) WriteJSON(path string) error {
	_, err := df.inner.WriteJson(pyffi.KW{"file": path})
	return err
}

// ToJSON returns JSON string.
func (df *DataFrame) ToJSON() (string, error) {
	fn := df.inner.Object().Attr("write_json")
	if fn == nil {
		return "", fmt.Errorf("polarsgo: write_json not found")
	}
	defer fn.Close()
	result, err := fn.Call()
	if err != nil {
		return "", err
	}
	defer result.Close()
	return result.GoString()
}

// --- Expression-based DataFrame methods (variadic *exprs in Python) ---

// Filter filters rows using a Polars expression string.
func (df *DataFrame) Filter(expr string) (*DataFrame, error) {
	return df.execExpr(fmt.Sprintf("_pl_result = _pl_df.filter(%s)", expr))
}

// Select selects columns by expression.
func (df *DataFrame) Select(exprs ...string) (*DataFrame, error) {
	return df.execExpr(fmt.Sprintf("_pl_result = _pl_df.select(%s)", joinExprs(exprs)))
}

// SelectCols selects columns by name.
func (df *DataFrame) SelectCols(cols ...string) (*DataFrame, error) {
	return df.execExpr(fmt.Sprintf("_pl_result = _pl_df.select(%s)", pyStrList(cols)))
}

// Drop removes columns by name.
func (df *DataFrame) Drop(cols ...string) (*DataFrame, error) {
	return df.execExpr(fmt.Sprintf("_pl_result = _pl_df.drop(%s)", pyStrList(cols)))
}

// WithColumns adds or transforms columns.
func (df *DataFrame) WithColumns(exprs ...string) (*DataFrame, error) {
	return df.execExpr(fmt.Sprintf("_pl_result = _pl_df.with_columns(%s)", joinExprs(exprs)))
}

// GroupBy groups by columns.
func (df *DataFrame) GroupBy(cols ...string) *GroupBy {
	return &GroupBy{rt: df.rt, df: df.inner, cols: cols}
}

// Pivot creates a pivot table.
func (df *DataFrame) Pivot(on, index, values string) (*DataFrame, error) {
	return df.execExpr(fmt.Sprintf("_pl_result = _pl_df.pivot(on=%q, index=%q, values=%q)", on, index, values))
}

// Unpivot converts wide to long format.
func (df *DataFrame) Unpivot(on, index []string) (*DataFrame, error) {
	return df.execExpr(fmt.Sprintf("_pl_result = _pl_df.unpivot(on=%s, index=%s)", pyStrList(on), pyStrList(index)))
}

func (df *DataFrame) execExpr(code string) (*DataFrame, error) {
	mainMod, _ := df.rt.Import("__main__")
	defer mainMod.Close()
	mainMod.SetAttr("_pl_df", df.inner.Object())
	if err := df.rt.Exec("import polars as pl\nfrom polars import col\n" + code); err != nil {
		return nil, fmt.Errorf("polarsgo: %w", err)
	}
	result := mainMod.Attr("_pl_result")
	if result == nil {
		return nil, fmt.Errorf("polarsgo: no result")
	}
	return &DataFrame{inner: polars.WrapDataFrame(result, df.rt), rt: df.rt}, nil
}

// --- LazyFrame ---

// LazyFrame wraps a Polars LazyFrame for deferred execution.
type LazyFrame struct {
	inner *polars.LazyFrame
	rt    *pyffi.Runtime
}

func wrapLF(rt *pyffi.Runtime, lf *polars.LazyFrame) *LazyFrame {
	return &LazyFrame{inner: lf, rt: rt}
}

// Close releases the LazyFrame.
func (lf *LazyFrame) Close() error { return lf.inner.Close() }

// Object returns the underlying Python object.
func (lf *LazyFrame) Object() *pyffi.Object { return lf.inner.Object() }

// Collect executes the lazy plan and returns a DataFrame.
func (lf *LazyFrame) Collect() (*DataFrame, error) {
	mainMod, _ := lf.rt.Import("__main__")
	defer mainMod.Close()
	mainMod.SetAttr("_pl_lf", lf.inner.Object())
	if err := lf.rt.Exec("_pl_result = _pl_lf.collect()"); err != nil {
		return nil, fmt.Errorf("polarsgo: collect: %w", err)
	}
	result := mainMod.Attr("_pl_result")
	if result == nil {
		return nil, fmt.Errorf("polarsgo: collect: no result")
	}
	return &DataFrame{inner: polars.WrapDataFrame(result, lf.rt), rt: lf.rt}, nil
}

// Explain returns the query plan (generated binding).
func (lf *LazyFrame) Explain() string {
	s, err := lf.inner.Explain()
	if err != nil {
		return ""
	}
	return s
}

// Limit limits rows (generated binding).
func (lf *LazyFrame) Limit(n int) *LazyFrame {
	result, err := lf.inner.Limit(pyffi.KW{"n": n})
	if err != nil {
		return lf
	}
	return wrapLF(lf.rt, result)
}

// Sort sorts by column.
func (lf *LazyFrame) Sort(col string, descending bool) *LazyFrame {
	result, err := lf.inner.Sort(col, pyffi.KW{"descending": descending})
	if err != nil {
		return lf
	}
	return wrapLF(lf.rt, result)
}

// Unique returns unique rows (generated binding).
func (lf *LazyFrame) Unique(cols ...string) *LazyFrame {
	if len(cols) > 0 {
		s := make([]any, len(cols))
		for i, c := range cols {
			s[i] = c
		}
		result, err := lf.inner.Unique(pyffi.KW{"subset": s})
		if err != nil {
			return lf
		}
		return wrapLF(lf.rt, result)
	}
	result, err := lf.inner.Unique()
	if err != nil {
		return lf
	}
	return wrapLF(lf.rt, result)
}

// DropNulls removes null rows (generated binding).
func (lf *LazyFrame) DropNulls() *LazyFrame {
	result, err := lf.inner.DropNulls()
	if err != nil {
		return lf
	}
	return wrapLF(lf.rt, result)
}

// Join joins with another LazyFrame (generated binding).
func (lf *LazyFrame) Join(other *LazyFrame, on string, how string) *LazyFrame {
	result, err := lf.inner.Join(other.inner, pyffi.KW{"on": on, "how": how})
	if err != nil {
		return lf
	}
	return wrapLF(lf.rt, result)
}

// Filter filters rows using a Polars expression string.
func (lf *LazyFrame) Filter(expr string) *LazyFrame {
	return lf.chainExpr(fmt.Sprintf("_pl_lf_result = _pl_lf.filter(%s)", expr))
}

// Select selects columns by expression.
func (lf *LazyFrame) Select(exprs ...string) *LazyFrame {
	return lf.chainExpr(fmt.Sprintf("_pl_lf_result = _pl_lf.select(%s)", joinExprs(exprs)))
}

// WithColumns adds or transforms columns.
func (lf *LazyFrame) WithColumns(exprs ...string) *LazyFrame {
	return lf.chainExpr(fmt.Sprintf("_pl_lf_result = _pl_lf.with_columns(%s)", joinExprs(exprs)))
}

// Rename renames columns.
func (lf *LazyFrame) Rename(pairs ...string) *LazyFrame {
	if len(pairs)%2 != 0 {
		return lf
	}
	mapping := "{"
	for i := 0; i < len(pairs); i += 2 {
		if i > 0 {
			mapping += ", "
		}
		mapping += fmt.Sprintf("%q: %q", pairs[i], pairs[i+1])
	}
	mapping += "}"
	return lf.chainExpr(fmt.Sprintf("_pl_lf_result = _pl_lf.rename(%s)", mapping))
}

// GroupBy groups and aggregates.
func (lf *LazyFrame) GroupBy(cols ...string) *LazyGroupBy {
	return &LazyGroupBy{rt: lf.rt, lf: lf.inner, cols: cols}
}

func (lf *LazyFrame) chainExpr(expr string) *LazyFrame {
	mainMod, _ := lf.rt.Import("__main__")
	defer mainMod.Close()
	mainMod.SetAttr("_pl_lf", lf.inner.Object())
	if err := lf.rt.Exec("import polars as pl\nfrom polars import col\n" + expr); err != nil {
		return lf
	}
	result := mainMod.Attr("_pl_lf_result")
	if result == nil {
		return lf
	}
	return &LazyFrame{inner: polars.WrapLazyFrame(result, lf.rt), rt: lf.rt}
}

// --- GroupBy ---

// GroupBy represents a grouped DataFrame.
type GroupBy struct {
	rt   *pyffi.Runtime
	df   *polars.DataFrame
	cols []string
}

// Agg performs aggregation with expression strings.
func (gb *GroupBy) Agg(exprs ...string) (*DataFrame, error) {
	mainMod, _ := gb.rt.Import("__main__")
	defer mainMod.Close()
	mainMod.SetAttr("_pl_df", gb.df.Object())
	code := fmt.Sprintf("import polars as pl\nfrom polars import col\n_pl_result = _pl_df.group_by(%s).agg(%s)",
		pyStrList(gb.cols), joinExprs(exprs))
	if err := gb.rt.Exec(code); err != nil {
		return nil, fmt.Errorf("polarsgo: group_by.agg: %w", err)
	}
	result := mainMod.Attr("_pl_result")
	if result == nil {
		return nil, fmt.Errorf("polarsgo: group_by.agg: no result")
	}
	return &DataFrame{inner: polars.WrapDataFrame(result, gb.rt), rt: gb.rt}, nil
}

// LazyGroupBy represents a grouped LazyFrame.
type LazyGroupBy struct {
	rt   *pyffi.Runtime
	lf   *polars.LazyFrame
	cols []string
}

// Agg performs aggregation.
func (lgb *LazyGroupBy) Agg(exprs ...string) *LazyFrame {
	mainMod, _ := lgb.rt.Import("__main__")
	defer mainMod.Close()
	mainMod.SetAttr("_pl_lf", lgb.lf.Object())
	code := fmt.Sprintf("import polars as pl\nfrom polars import col\n_pl_lf_result = _pl_lf.group_by(%s).agg(%s)",
		pyStrList(lgb.cols), joinExprs(exprs))
	if err := lgb.rt.Exec(code); err != nil {
		return &LazyFrame{inner: lgb.lf, rt: lgb.rt}
	}
	result := mainMod.Attr("_pl_lf_result")
	if result == nil {
		return &LazyFrame{inner: lgb.lf, rt: lgb.rt}
	}
	return &LazyFrame{inner: polars.WrapLazyFrame(result, lgb.rt), rt: lgb.rt}
}

// --- IO (generated Module methods) ---

// ReadCSV reads a CSV file (generated binding).
func (c *Client) ReadCSV(path string) (*DataFrame, error) {
	result, err := c.mod.ReadCsv(path)
	if err != nil {
		return nil, fmt.Errorf("polarsgo: read_csv: %w", err)
	}
	return wrapDF(c.rt, result), nil
}

// ReadJSON reads a JSON file (generated binding).
func (c *Client) ReadJSON(path string) (*DataFrame, error) {
	result, err := c.mod.ReadJson(path)
	if err != nil {
		return nil, fmt.Errorf("polarsgo: read_json: %w", err)
	}
	return wrapDF(c.rt, result), nil
}

// ReadParquet reads a Parquet file (generated binding).
func (c *Client) ReadParquet(path string) (*DataFrame, error) {
	result, err := c.mod.ReadParquet(path)
	if err != nil {
		return nil, fmt.Errorf("polarsgo: read_parquet: %w", err)
	}
	return wrapDF(c.rt, result), nil
}

// FromMaps creates a DataFrame from a slice of maps.
func (c *Client) FromMaps(data []map[string]any) (*DataFrame, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	code := fmt.Sprintf("import polars as pl, json\n_pl_result = pl.DataFrame(json.loads(%q))", string(b))
	if err := c.rt.Exec(code); err != nil {
		return nil, fmt.Errorf("polarsgo: from_maps: %w", err)
	}
	mainMod, _ := c.rt.Import("__main__")
	defer mainMod.Close()
	result := mainMod.Attr("_pl_result")
	if result == nil {
		return nil, fmt.Errorf("polarsgo: from_maps: no result")
	}
	return &DataFrame{inner: polars.WrapDataFrame(result, c.rt), rt: c.rt}, nil
}

// Concat concatenates multiple DataFrames.
func (c *Client) Concat(dfs ...*DataFrame) (*DataFrame, error) {
	if len(dfs) == 0 {
		return nil, fmt.Errorf("polarsgo: concat: no DataFrames")
	}
	mainMod, _ := c.rt.Import("__main__")
	defer mainMod.Close()
	for i, df := range dfs {
		mainMod.SetAttr(fmt.Sprintf("_pl_concat_%d", i), df.inner.Object())
	}
	var refs []string
	for i := range dfs {
		refs = append(refs, fmt.Sprintf("_pl_concat_%d", i))
	}
	code := fmt.Sprintf("import polars as pl\n_pl_result = pl.concat([%s])", joinComma(refs))
	if err := c.rt.Exec(code); err != nil {
		return nil, fmt.Errorf("polarsgo: concat: %w", err)
	}
	result := mainMod.Attr("_pl_result")
	if result == nil {
		return nil, fmt.Errorf("polarsgo: concat: no result")
	}
	return &DataFrame{inner: polars.WrapDataFrame(result, c.rt), rt: c.rt}, nil
}

// SQL executes a SQL query against registered DataFrames.
func (c *Client) SQL(query string, tables map[string]*DataFrame) (*DataFrame, error) {
	mainMod, _ := c.rt.Import("__main__")
	defer mainMod.Close()
	var regCode string
	regCode += "import polars as pl\n_pl_ctx = pl.SQLContext()\n"
	for name, df := range tables {
		attr := fmt.Sprintf("_pl_sql_%s", name)
		mainMod.SetAttr(attr, df.inner.Object())
		regCode += fmt.Sprintf("_pl_ctx.register(%q, %s)\n", name, attr)
	}
	regCode += fmt.Sprintf("_pl_result = _pl_ctx.execute(%q).collect()\n", query)
	if err := c.rt.Exec(regCode); err != nil {
		return nil, fmt.Errorf("polarsgo: sql: %w", err)
	}
	result := mainMod.Attr("_pl_result")
	if result == nil {
		return nil, fmt.Errorf("polarsgo: sql: no result")
	}
	return &DataFrame{inner: polars.WrapDataFrame(result, c.rt), rt: c.rt}, nil
}

// --- Helpers ---

func pyStrList(items []string) string {
	s := "["
	for i, item := range items {
		if i > 0 {
			s += ", "
		}
		s += fmt.Sprintf("%q", item)
	}
	s += "]"
	return s
}

func joinExprs(exprs []string) string {
	result := ""
	for i, e := range exprs {
		if i > 0 {
			result += ", "
		}
		result += e
	}
	return result
}

func joinComma(items []string) string {
	result := ""
	for i, item := range items {
		if i > 0 {
			result += ", "
		}
		result += item
	}
	return result
}
