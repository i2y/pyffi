package pyffi_test

import (
	"testing"

	"github.com/i2y/pyffi"
)

// Benchmarks use the shared Runtime from TestMain.
// With auto-GIL, no explicit WithGIL wrappers are needed.

func ensureBenchRT(b *testing.B) *pyffi.Runtime {
	b.Helper()
	if sharedRT == nil {
		b.Skip("Python not available")
	}
	return sharedRT
}

func BenchmarkExec(b *testing.B) {
	rt := ensureBenchRT(b)
	for b.Loop() {
		rt.Exec("x = 1 + 1")
	}
}

func BenchmarkCallFunction(b *testing.B) {
	rt := ensureBenchRT(b)
	builtinsObj, _ := rt.Import("builtins")
	lenFnObj := builtinsObj.Attr("len")
	listObj, _ := rt.FromGoValue([]any{int64(1), int64(2), int64(3)})
	b.Cleanup(func() {
		listObj.Close()
		lenFnObj.Close()
		builtinsObj.Close()
	})
	for b.Loop() {
		result, _ := lenFnObj.Call(listObj)
		result.Close()
	}
}

func BenchmarkTypeConvertInt(b *testing.B) {
	rt := ensureBenchRT(b)
	for b.Loop() {
		obj := rt.FromInt64(42)
		obj.Int64()
		obj.Close()
	}
}

func BenchmarkTypeConvertString(b *testing.B) {
	rt := ensureBenchRT(b)
	for b.Loop() {
		obj := rt.FromString("hello world")
		obj.GoString()
		obj.Close()
	}
}

func BenchmarkGetItem(b *testing.B) {
	rt := ensureBenchRT(b)
	dict, _ := rt.NewDict("key", "value")
	b.Cleanup(func() { dict.Close() })
	for b.Loop() {
		v, _ := dict.GetItem("key")
		v.Close()
	}
}

func BenchmarkIter(b *testing.B) {
	rt := ensureBenchRT(b)
	list, _ := rt.NewList(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	b.Cleanup(func() { list.Close() })
	for b.Loop() {
		iter, _ := list.Iter()
		for {
			item, _ := iter.Next()
			if item == nil {
				break
			}
			item.Close()
		}
		iter.Close()
	}
}

func BenchmarkListRoundTrip(b *testing.B) {
	rt := ensureBenchRT(b)
	input := []any{int64(1), int64(2), int64(3), "four", 5.0}
	for b.Loop() {
		obj, _ := rt.FromGoValue(input)
		obj.GoValue()
		obj.Close()
	}
}

func BenchmarkEval(b *testing.B) {
	rt := ensureBenchRT(b)
	for b.Loop() {
		r, _ := rt.Eval("1 + 2")
		r.Close()
	}
}
