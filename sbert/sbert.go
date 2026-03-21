// Package sbert provides Go bindings for [sentence-transformers],
// enabling semantic embeddings and similarity computation from Go.
//
// The Python package is auto-installed via uv on first use.
//
//	model, _ := sbert.New("all-MiniLM-L6-v2")
//	defer model.Close()
//
//	embeddings, _ := model.Encode([]string{"Hello world", "Go is great"})
//	sim, _ := model.Similarity(embeddings, embeddings)
//
// [sentence-transformers]: https://www.sbert.net
package sbert

import (
	"fmt"

	"github.com/i2y/pyffi"
)

// Model wraps a sentence-transformers SentenceTransformer instance.
type Model struct {
	rt    *pyffi.Runtime
	model *pyffi.Object
	owns  bool // true if Model created its own Runtime
}

// New creates a new SentenceTransformer model.
// The sentence-transformers package is auto-installed via uv.
//
//	model, _ := sbert.New("all-MiniLM-L6-v2")
//	defer model.Close()
func New(modelName string) (*Model, error) {
	rt, err := pyffi.New(pyffi.Dependencies("sentence-transformers"))
	if err != nil {
		return nil, fmt.Errorf("sbert: %w", err)
	}
	m, err := NewWithRuntime(rt, modelName)
	if err != nil {
		rt.Close()
		return nil, err
	}
	m.owns = true
	return m, nil
}

// NewWithRuntime creates a SentenceTransformer using an existing Runtime.
// The caller is responsible for closing the Runtime.
func NewWithRuntime(rt *pyffi.Runtime, modelName string) (*Model, error) {
	stMod, err := rt.Import("sentence_transformers")
	if err != nil {
		return nil, fmt.Errorf("sbert: import: %w", err)
	}
	defer stMod.Close()

	cls := stMod.Attr("SentenceTransformer")
	if cls == nil {
		return nil, fmt.Errorf("sbert: SentenceTransformer class not found")
	}
	defer cls.Close()

	obj, err := cls.Call(modelName)
	if err != nil {
		return nil, fmt.Errorf("sbert: load model %q: %w", modelName, err)
	}

	return &Model{rt: rt, model: obj}, nil
}

// Close releases the model and (if created by New) the Runtime.
func (m *Model) Close() error {
	m.model.Close()
	if m.owns {
		return m.rt.Close()
	}
	return nil
}

// Encode converts sentences to embedding vectors.
// Returns a 2D slice where each row is the embedding for the corresponding sentence.
func (m *Model) Encode(sentences []string) ([][]float32, error) {
	// Convert Go string slice to Python list
	args := make([]any, len(sentences))
	for i, s := range sentences {
		args[i] = s
	}
	listObj, err := m.rt.NewList(args...)
	if err != nil {
		return nil, fmt.Errorf("sbert: create list: %w", err)
	}
	defer listObj.Close()

	encodeFn := m.model.Attr("encode")
	if encodeFn == nil {
		return nil, fmt.Errorf("sbert: encode method not found")
	}
	defer encodeFn.Close()

	resultObj, err := encodeFn.Call(listObj)
	if err != nil {
		return nil, fmt.Errorf("sbert: encode: %w", err)
	}
	defer resultObj.Close()

	return ndarrayToFloat32(m.rt, resultObj)
}

// Similarity computes pairwise similarity between two sets of embeddings.
// Returns a 2D similarity matrix.
func (m *Model) Similarity(a, b [][]float32) ([][]float32, error) {
	aObj, err := float32ToList(m.rt, a)
	if err != nil {
		return nil, err
	}
	defer aObj.Close()

	bObj, err := float32ToList(m.rt, b)
	if err != nil {
		return nil, err
	}
	defer bObj.Close()

	simFn := m.model.Attr("similarity")
	if simFn == nil {
		return nil, fmt.Errorf("sbert: similarity method not found")
	}
	defer simFn.Close()

	resultObj, err := simFn.Call(aObj, bObj)
	if err != nil {
		return nil, fmt.Errorf("sbert: similarity: %w", err)
	}
	defer resultObj.Close()

	return tensorToFloat32(m.rt, resultObj)
}

// ndarrayToFloat32 converts a numpy ndarray to [][]float32 via .tolist().
func ndarrayToFloat32(rt *pyffi.Runtime, obj *pyffi.Object) ([][]float32, error) {
	toListFn := obj.Attr("tolist")
	if toListFn == nil {
		return nil, fmt.Errorf("sbert: tolist not found")
	}
	defer toListFn.Close()

	listObj, err := toListFn.Call()
	if err != nil {
		return nil, fmt.Errorf("sbert: tolist: %w", err)
	}
	defer listObj.Close()

	return pyListToFloat32_2D(listObj)
}

// tensorToFloat32 converts a PyTorch tensor to [][]float32 via .tolist().
func tensorToFloat32(rt *pyffi.Runtime, obj *pyffi.Object) ([][]float32, error) {
	toListFn := obj.Attr("tolist")
	if toListFn == nil {
		return nil, fmt.Errorf("sbert: tolist not found")
	}
	defer toListFn.Close()

	listObj, err := toListFn.Call()
	if err != nil {
		return nil, fmt.Errorf("sbert: tolist: %w", err)
	}
	defer listObj.Close()

	return pyListToFloat32_2D(listObj)
}

// pyListToFloat32_2D converts a Python list of lists to [][]float32.
func pyListToFloat32_2D(listObj *pyffi.Object) ([][]float32, error) {
	n, _ := listObj.Len()
	if n <= 0 {
		return nil, nil
	}

	result := make([][]float32, n)
	for i := int64(0); i < n; i++ {
		row, err := listObj.GetItem(int(i))
		if err != nil {
			return nil, err
		}

		m, _ := row.Len()
		result[i] = make([]float32, m)
		for j := int64(0); j < m; j++ {
			val, err := row.GetItem(int(j))
			if err != nil {
				row.Close()
				return nil, err
			}
			f, _ := val.Float64()
			result[i][j] = float32(f)
			val.Close()
		}
		row.Close()
	}

	return result, nil
}

// float32ToList converts [][]float32 to a Python list of lists.
func float32ToList(rt *pyffi.Runtime, data [][]float32) (*pyffi.Object, error) {
	rows := make([]any, len(data))
	for i, row := range data {
		cols := make([]any, len(row))
		for j, v := range row {
			cols[j] = float64(v)
		}
		list, err := rt.NewList(cols...)
		if err != nil {
			return nil, err
		}
		rows[i] = list
	}
	return rt.NewList(rows...)
}
