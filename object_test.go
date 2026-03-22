package pyffi_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/i2y/pyffi"
)

// --- Object lifecycle ---

func TestObjectCloseNil(t *testing.T) {
	var o *pyffi.Object
	if err := o.Close(); err != nil {
		t.Fatalf("Close on nil Object: %v", err)
	}
}

func TestObjectDoubleClose(t *testing.T) {
	rt := newOrSkip(t)

	obj := rt.FromInt64(42)
	if err := obj.Close(); err != nil {
		t.Fatal(err)
	}
	if err := obj.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestObjectString(t *testing.T) {
	rt := newOrSkip(t)

	obj := rt.FromInt64(123)
	defer obj.Close()
	if got := obj.String(); got != "123" {
		t.Fatalf("got %q, want %q", got, "123")
	}
}

func TestObjectIsNone(t *testing.T) {
	rt := newOrSkip(t)

	none := rt.None()
	defer none.Close()
	if !none.IsNone() {
		t.Fatal("expected IsNone to be true")
	}

	obj := rt.FromInt64(1)
	defer obj.Close()
	if obj.IsNone() {
		t.Fatal("expected IsNone to be false for int")
	}
}

// --- Type conversions ---

func TestConvertInt(t *testing.T) {
	rt := newOrSkip(t)

	obj := rt.FromInt64(42)
	defer obj.Close()

	got, err := obj.Int64()
	if err != nil {
		t.Fatal(err)
	}
	if got != 42 {
		t.Fatalf("got %d, want 42", got)
	}
}

func TestConvertIntNegative(t *testing.T) {
	rt := newOrSkip(t)

	obj := rt.FromInt64(-100)
	defer obj.Close()

	got, err := obj.Int64()
	if err != nil {
		t.Fatal(err)
	}
	if got != -100 {
		t.Fatalf("got %d, want -100", got)
	}
}

func TestConvertFloat(t *testing.T) {
	rt := newOrSkip(t)

	obj := rt.FromFloat64(3.14)
	defer obj.Close()

	got, err := obj.Float64()
	if err != nil {
		t.Fatal(err)
	}
	if got != 3.14 {
		t.Fatalf("got %f, want 3.14", got)
	}
}

func TestConvertString(t *testing.T) {
	rt := newOrSkip(t)

	obj := rt.FromString("hello")
	defer obj.Close()

	got, err := obj.GoString()
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello" {
		t.Fatalf("got %q, want %q", got, "hello")
	}
}

func TestConvertStringJapanese(t *testing.T) {
	rt := newOrSkip(t)

	obj := rt.FromString("こんにちは世界")
	defer obj.Close()

	got, err := obj.GoString()
	if err != nil {
		t.Fatal(err)
	}
	if got != "こんにちは世界" {
		t.Fatalf("got %q, want %q", got, "こんにちは世界")
	}
}

func TestConvertBoolTrue(t *testing.T) {
	rt := newOrSkip(t)

	obj := rt.FromBool(true)
	defer obj.Close()

	got, err := obj.Bool()
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatal("got false, want true")
	}
}

func TestConvertBoolFalse(t *testing.T) {
	rt := newOrSkip(t)

	obj := rt.FromBool(false)
	defer obj.Close()

	got, err := obj.Bool()
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Fatal("got true, want false")
	}
}

func TestConvertList(t *testing.T) {
	rt := newOrSkip(t)

	input := []any{int64(1), int64(2), int64(3)}
	obj, err := rt.FromGoValue(input)
	if err != nil {
		t.Fatal(err)
	}
	defer obj.Close()

	got, err := obj.GoSlice()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got len %d, want 3", len(got))
	}
	for i, want := range []int64{1, 2, 3} {
		if got[i] != want {
			t.Fatalf("got[%d] = %v, want %d", i, got[i], want)
		}
	}
}

func TestConvertDict(t *testing.T) {
	rt := newOrSkip(t)

	input := map[string]any{"key": "value", "num": int64(42)}
	obj, err := rt.FromGoValue(input)
	if err != nil {
		t.Fatal(err)
	}
	defer obj.Close()

	got, err := obj.GoMap()
	if err != nil {
		t.Fatal(err)
	}
	if got["key"] != "value" {
		t.Fatalf("got[key] = %v, want %q", got["key"], "value")
	}
	if got["num"] != int64(42) {
		t.Fatalf("got[num] = %v, want 42", got["num"])
	}
}

func TestConvertNested(t *testing.T) {
	rt := newOrSkip(t)

	input := []any{int64(1), []any{int64(2), int64(3)}}
	obj, err := rt.FromGoValue(input)
	if err != nil {
		t.Fatal(err)
	}
	defer obj.Close()

	got, err := obj.GoSlice()
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != int64(1) {
		t.Fatalf("got[0] = %v, want 1", got[0])
	}
	inner, ok := got[1].([]any)
	if !ok {
		t.Fatalf("got[1] type = %T, want []any", got[1])
	}
	if inner[0] != int64(2) || inner[1] != int64(3) {
		t.Fatalf("inner = %v, want [2, 3]", inner)
	}
}

func TestConvertNil(t *testing.T) {
	rt := newOrSkip(t)

	obj, err := rt.FromGoValue(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer obj.Close()

	if !obj.IsNone() {
		t.Fatal("expected None")
	}

	val, err := obj.GoValue()
	if err != nil {
		t.Fatal(err)
	}
	if val != nil {
		t.Fatalf("got %v, want nil", val)
	}
}

func TestGoValueAutoDetect(t *testing.T) {
	rt := newOrSkip(t)

	// bool should round-trip as bool (not int)
	obj := rt.FromBool(true)
	defer obj.Close()

	val, err := obj.GoValue()
	if err != nil {
		t.Fatal(err)
	}
	b, ok := val.(bool)
	if !ok {
		t.Fatalf("expected bool, got %T", val)
	}
	if !b {
		t.Fatal("expected true")
	}
}

// --- Import ---

func TestImportObject(t *testing.T) {
	rt := newOrSkip(t)

	sys, err := rt.Import("sys")
	if err != nil {
		t.Fatal(err)
	}
	defer sys.Close()

	version := sys.Attr("version")
	if version == nil {
		t.Fatal("sys.version is nil")
	}
	defer version.Close()

	s, err := version.GoString()
	if err != nil {
		t.Fatal(err)
	}
	if s == "" {
		t.Fatal("sys.version is empty")
	}
	t.Logf("Python version: %s", s)
}

// --- Attribute access ---

func TestAttrChain(t *testing.T) {
	rt := newOrSkip(t)

	os, err := rt.Import("os")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Close()

	join := os.Attr("path").Attr("join")
	if join == nil {
		t.Fatal("os.path.join is nil")
	}
	defer join.Close()

	result, err := join.Call("a", "b", "c")
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()

	s, err := result.GoString()
	if err != nil {
		t.Fatal(err)
	}
	// os.path.join uses "/" on Unix and "\" on Windows.
	if s != "a/b/c" && s != `a\b\c` {
		t.Fatalf("got %q, want %q or %q", s, "a/b/c", `a\b\c`)
	}
}

func TestAttrNotFound(t *testing.T) {
	rt := newOrSkip(t)

	sys, err := rt.Import("sys")
	if err != nil {
		t.Fatal(err)
	}
	defer sys.Close()

	result := sys.Attr("nonexistent_attribute_xyz")
	if result != nil {
		result.Close()
		t.Fatal("expected nil for nonexistent attribute")
	}
}

func TestAttrErr(t *testing.T) {
	rt := newOrSkip(t)

	sys, err := rt.Import("sys")
	if err != nil {
		t.Fatal(err)
	}
	defer sys.Close()

	_, err = sys.AttrErr("nonexistent_attribute_xyz")
	if err == nil {
		t.Fatal("expected error")
	}
	var pyErr *pyffi.PythonError
	if !errors.As(err, &pyErr) {
		t.Fatalf("expected *PythonError, got %T: %v", err, err)
	}
	if pyErr.Type != "AttributeError" {
		t.Fatalf("expected AttributeError, got %q", pyErr.Type)
	}
}

// --- Function calls ---

func TestCallBuiltin(t *testing.T) {
	rt := newOrSkip(t)

	builtins, err := rt.Import("builtins")
	if err != nil {
		t.Fatal(err)
	}
	defer builtins.Close()

	lenFunc := builtins.Attr("len")
	if lenFunc == nil {
		t.Fatal("builtins.len is nil")
	}
	defer lenFunc.Close()

	list, err := rt.FromGoValue([]any{int64(1), int64(2), int64(3)})
	if err != nil {
		t.Fatal(err)
	}
	defer list.Close()

	result, err := lenFunc.Call(list)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()

	n, err := result.Int64()
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("len = %d, want 3", n)
	}
}

func TestCallWithKwargs(t *testing.T) {
	rt := newOrSkip(t)

	json, err := rt.Import("json")
	if err != nil {
		t.Fatal(err)
	}
	defer json.Close()

	dumps := json.Attr("dumps")
	if dumps == nil {
		t.Fatal("json.dumps is nil")
	}
	defer dumps.Close()

	data, err := rt.FromGoValue(map[string]any{"a": int64(1)})
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()

	result, err := dumps.Call(data, pyffi.KW{"sort_keys": true})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()

	s, err := result.GoString()
	if err != nil {
		t.Fatal(err)
	}
	if s != `{"a": 1}` {
		t.Fatalf("got %q, want %q", s, `{"a": 1}`)
	}
}

func TestCallError(t *testing.T) {
	rt := newOrSkip(t)

	obj := rt.FromInt64(42)
	defer obj.Close()

	_, err := obj.Call()
	if err == nil {
		t.Fatal("expected error calling non-callable")
	}
	var pyErr *pyffi.PythonError
	if !errors.As(err, &pyErr) {
		t.Fatalf("expected *PythonError, got %T: %v", err, err)
	}
}

// --- Error handling ---

func TestPythonErrorMessage(t *testing.T) {
	rt := newOrSkip(t)

	_, err := rt.Import("nonexistent_module_xyz_12345")
	if err == nil {
		t.Fatal("expected error")
	}
	var pyErr *pyffi.PythonError
	if !errors.As(err, &pyErr) {
		t.Fatalf("expected *PythonError, got %T: %v", err, err)
	}
	if pyErr.Type != "ModuleNotFoundError" {
		t.Fatalf("expected ModuleNotFoundError, got %q", pyErr.Type)
	}
	t.Logf("Error: %v", err)
}

// --- GIL ---

func TestWithGIL(t *testing.T) {
	rt := newOrSkip(t)

	err := rt.WithGIL(func() error {
		return rt.Exec("x = 42")
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestGILMultiGoroutine(t *testing.T) {
	rt := newOrSkip(t)

	// With auto-GIL, goroutines can call Python methods directly.
	var wg sync.WaitGroup
	errs := make([]error, 10)

	for i := range 10 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			obj := rt.FromInt64(int64(idx))
			defer obj.Close()
			v, err := obj.Int64()
			if err != nil {
				errs[idx] = err
				return
			}
			if v != int64(idx) {
				errs[idx] = errors.New("value mismatch")
			}
		}(i)
	}

	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}
}
