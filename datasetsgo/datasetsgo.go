// Package datasetsgo provides Go bindings for [Hugging Face Datasets],
// enabling access to 200,000+ datasets on the Hub from Go.
//
//	ds, _ := datasetsgo.Load("imdb")
//	defer ds.Close()
//
//	train := ds.Split("train")
//	fmt.Println(train.Len())
//	row, _ := train.Row(0)
//	fmt.Println(row["text"])
//
// [Hugging Face Datasets]: https://github.com/huggingface/datasets
package datasetsgo

import (
	"encoding/json"
	"fmt"

	"github.com/i2y/pyffi"
)

// Client manages a datasets runtime.
type Client struct {
	rt  *pyffi.Runtime
	mod *pyffi.Object
}

// New creates a new datasets client.
func New() (*Client, error) {
	rt, err := pyffi.New(pyffi.Dependencies("datasets"))
	if err != nil {
		return nil, fmt.Errorf("datasetsgo: %w", err)
	}
	mod, err := rt.Import("datasets")
	if err != nil {
		rt.Close()
		return nil, fmt.Errorf("datasetsgo: import: %w", err)
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

// DatasetDict wraps a datasets.DatasetDict (multiple splits).
type DatasetDict struct {
	rt  *pyffi.Runtime
	obj *pyffi.Object
}

// Load loads a dataset from the Hugging Face Hub.
func (c *Client) Load(name string, opts ...LoadOption) (*DatasetDict, error) {
	var cfg loadConfig
	for _, o := range opts {
		o(&cfg)
	}

	fn := c.mod.Attr("load_dataset")
	if fn == nil {
		return nil, fmt.Errorf("datasetsgo: load_dataset not found")
	}
	defer fn.Close()

	kw := pyffi.KW{}
	if cfg.split != "" {
		kw["split"] = cfg.split
	}
	if cfg.streaming {
		kw["streaming"] = true
	}
	if cfg.subset != "" {
		kw["name"] = cfg.subset
	}

	var result *pyffi.Object
	var err error
	if len(kw) > 0 {
		result, err = fn.Call(name, kw)
	} else {
		result, err = fn.Call(name)
	}
	if err != nil {
		return nil, fmt.Errorf("datasetsgo: load %q: %w", name, err)
	}

	return &DatasetDict{rt: c.rt, obj: result}, nil
}

// LoadOption configures Load.
type LoadOption func(*loadConfig)

type loadConfig struct {
	split     string
	streaming bool
	subset    string
}

// WithSplit loads a specific split (e.g. "train", "test").
func WithSplit(split string) LoadOption {
	return func(c *loadConfig) { c.split = split }
}

// WithStreaming enables streaming mode (no full download).
func WithStreaming() LoadOption {
	return func(c *loadConfig) { c.streaming = true }
}

// WithSubset selects a dataset subset/configuration.
func WithSubset(name string) LoadOption {
	return func(c *loadConfig) { c.subset = name }
}

// Close releases the DatasetDict.
func (dd *DatasetDict) Close() error { return dd.obj.Close() }

// Splits returns the available split names.
func (dd *DatasetDict) Splits() []string {
	// dict_keys is not a list, so convert via list()
	mainMod, _ := dd.rt.Import("__main__")
	defer mainMod.Close()
	mainMod.SetAttr("_ds_dd", dd.obj)
	if err := dd.rt.Exec("_ds_splits = list(_ds_dd.keys())"); err != nil {
		return nil
	}
	result := mainMod.Attr("_ds_splits")
	if result == nil {
		return nil
	}
	defer result.Close()
	sl, _ := result.GoSlice()
	var names []string
	for _, v := range sl {
		if s, ok := v.(string); ok {
			names = append(names, s)
		}
	}
	return names
}

// Split returns a specific split as a Dataset.
func (dd *DatasetDict) Split(name string) *Dataset {
	obj, err := dd.obj.GetItem(name)
	if err != nil || obj == nil {
		return nil
	}
	return &Dataset{rt: dd.rt, obj: obj}
}

// Dataset wraps a single dataset split.
type Dataset struct {
	rt  *pyffi.Runtime
	obj *pyffi.Object
}

// Close releases the Dataset.
func (ds *Dataset) Close() error { return ds.obj.Close() }

// Len returns the number of rows.
func (ds *Dataset) Len() int {
	n, _ := ds.obj.Len()
	return int(n)
}

// Columns returns the column names.
func (ds *Dataset) Columns() []string {
	colObj := ds.obj.Attr("column_names")
	if colObj == nil {
		return nil
	}
	defer colObj.Close()
	sl, _ := colObj.GoSlice()
	var names []string
	for _, v := range sl {
		if s, ok := v.(string); ok {
			names = append(names, s)
		}
	}
	return names
}

// Row returns a single row as a map.
func (ds *Dataset) Row(index int) (map[string]any, error) {
	rowObj, err := ds.obj.GetItem(index)
	if err != nil {
		return nil, fmt.Errorf("datasetsgo: row %d: %w", index, err)
	}
	defer rowObj.Close()
	return rowObj.GoMap()
}

// Select returns a subset of rows by indices.
func (ds *Dataset) Select(indices []int) (*Dataset, error) {
	args := make([]any, len(indices))
	for i, idx := range indices {
		args[i] = idx
	}
	listObj, err := ds.rt.NewList(args...)
	if err != nil {
		return nil, err
	}
	defer listObj.Close()

	fn := ds.obj.Attr("select")
	if fn == nil {
		return nil, fmt.Errorf("datasetsgo: select not found")
	}
	defer fn.Close()
	result, err := fn.Call(listObj)
	if err != nil {
		return nil, fmt.Errorf("datasetsgo: select: %w", err)
	}
	return &Dataset{rt: ds.rt, obj: result}, nil
}

// Filter filters rows using a Python lambda expression.
// Example: ds.Filter("lambda x: len(x['text']) > 100")
func (ds *Dataset) Filter(lambdaExpr string) (*Dataset, error) {
	mainMod, _ := ds.rt.Import("__main__")
	defer mainMod.Close()
	mainMod.SetAttr("_ds_obj", ds.obj)
	code := fmt.Sprintf("_ds_result = _ds_obj.filter(%s)", lambdaExpr)
	if err := ds.rt.Exec(code); err != nil {
		return nil, fmt.Errorf("datasetsgo: filter: %w", err)
	}
	result := mainMod.Attr("_ds_result")
	if result == nil {
		return nil, fmt.Errorf("datasetsgo: filter: no result")
	}
	return &Dataset{rt: ds.rt, obj: result}, nil
}

// Map applies a transformation function (Python lambda) to each row.
// Example: ds.Map("lambda x: {'length': len(x['text'])}")
func (ds *Dataset) Map(lambdaExpr string) (*Dataset, error) {
	mainMod, _ := ds.rt.Import("__main__")
	defer mainMod.Close()
	mainMod.SetAttr("_ds_obj", ds.obj)
	code := fmt.Sprintf("_ds_result = _ds_obj.map(%s)", lambdaExpr)
	if err := ds.rt.Exec(code); err != nil {
		return nil, fmt.Errorf("datasetsgo: map: %w", err)
	}
	result := mainMod.Attr("_ds_result")
	if result == nil {
		return nil, fmt.Errorf("datasetsgo: map: no result")
	}
	return &Dataset{rt: ds.rt, obj: result}, nil
}

// Shuffle returns a shuffled copy.
func (ds *Dataset) Shuffle(seed int) (*Dataset, error) {
	fn := ds.obj.Attr("shuffle")
	if fn == nil {
		return nil, fmt.Errorf("datasetsgo: shuffle not found")
	}
	defer fn.Close()
	result, err := fn.Call(pyffi.KW{"seed": seed})
	if err != nil {
		return nil, fmt.Errorf("datasetsgo: shuffle: %w", err)
	}
	return &Dataset{rt: ds.rt, obj: result}, nil
}

// Sort sorts by a column.
func (ds *Dataset) Sort(column string) (*Dataset, error) {
	fn := ds.obj.Attr("sort")
	if fn == nil {
		return nil, fmt.Errorf("datasetsgo: sort not found")
	}
	defer fn.Close()
	result, err := fn.Call(column)
	if err != nil {
		return nil, fmt.Errorf("datasetsgo: sort: %w", err)
	}
	return &Dataset{rt: ds.rt, obj: result}, nil
}

// ToJSON returns the dataset as a JSON string.
func (ds *Dataset) ToJSON() (string, error) {
	mainMod, _ := ds.rt.Import("__main__")
	defer mainMod.Close()
	mainMod.SetAttr("_ds_obj", ds.obj)
	code := `
import json as _json
_ds_result = _json.dumps([_ds_obj[i] for i in range(min(len(_ds_obj), 1000))], default=str)
`
	if err := ds.rt.Exec(code); err != nil {
		return "", fmt.Errorf("datasetsgo: to_json: %w", err)
	}
	result := mainMod.Attr("_ds_result")
	if result == nil {
		return "", fmt.Errorf("datasetsgo: to_json: no result")
	}
	defer result.Close()
	return result.GoString()
}

// ToMaps returns up to n rows as a slice of maps.
func (ds *Dataset) ToMaps(n int) ([]map[string]any, error) {
	mainMod, _ := ds.rt.Import("__main__")
	defer mainMod.Close()
	mainMod.SetAttr("_ds_obj", ds.obj)
	code := fmt.Sprintf(`
import json as _json
_ds_result = _json.dumps([_ds_obj[i] for i in range(min(len(_ds_obj), %d))], default=str)
`, n)
	if err := ds.rt.Exec(code); err != nil {
		return nil, fmt.Errorf("datasetsgo: to_maps: %w", err)
	}
	result := mainMod.Attr("_ds_result")
	if result == nil {
		return nil, fmt.Errorf("datasetsgo: to_maps: no result")
	}
	defer result.Close()
	jsonStr, _ := result.GoString()
	var maps []map[string]any
	json.Unmarshal([]byte(jsonStr), &maps)
	return maps, nil
}
