package pyffi_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/i2y/pyffi"
)

// TestEval consolidates Eval tests into a single Runtime.
func TestEval(t *testing.T) {
	rt := newOrSkip(t)

	t.Run("Arithmetic", func(t *testing.T) {
		result, err := rt.Eval("1 + 2 * 3")
		if err != nil {
			t.Fatalf("Eval failed: %v", err)
		}
		defer result.Close()

		val, err := result.Int64()
		if err != nil {
			t.Fatalf("Int64 failed: %v", err)
		}
		if val != 7 {
			t.Errorf("Eval result = %d, want 7", val)
		}
	})

	t.Run("String", func(t *testing.T) {
		result, err := rt.Eval(`"hello" + " " + "world"`)
		if err != nil {
			t.Fatalf("Eval failed: %v", err)
		}
		defer result.Close()

		val, err := result.GoString()
		if err != nil {
			t.Fatalf("GoString failed: %v", err)
		}
		if val != "hello world" {
			t.Errorf("Eval result = %q, want %q", val, "hello world")
		}
	})

	t.Run("List", func(t *testing.T) {
		result, err := rt.Eval("[x**2 for x in range(4)]")
		if err != nil {
			t.Fatalf("Eval failed: %v", err)
		}
		defer result.Close()

		val, err := result.GoValue()
		if err != nil {
			t.Fatalf("GoValue failed: %v", err)
		}
		items := val.([]any)
		if len(items) != 4 {
			t.Fatalf("list length = %d, want 4", len(items))
		}
		if items[3] != int64(9) {
			t.Errorf("items[3] = %v, want 9", items[3])
		}
	})

	t.Run("BuiltinFunction", func(t *testing.T) {
		result, err := rt.Eval("len([1,2,3])")
		if err != nil {
			t.Fatalf("Eval failed: %v", err)
		}
		defer result.Close()

		val, err := result.Int64()
		if err != nil {
			t.Fatalf("Int64 failed: %v", err)
		}
		if val != 3 {
			t.Errorf("Eval result = %d, want 3", val)
		}
	})

	t.Run("UsesMainNamespace", func(t *testing.T) {
		// Variables set by Exec should be accessible in Eval.
		if err := rt.Exec("x = 42"); err != nil {
			t.Fatalf("Exec failed: %v", err)
		}

		result, err := rt.Eval("x * 2")
		if err != nil {
			t.Fatalf("Eval failed: %v", err)
		}
		defer result.Close()

		val, err := result.Int64()
		if err != nil {
			t.Fatalf("Int64 failed: %v", err)
		}
		if val != 84 {
			t.Errorf("Eval result = %d, want 84", val)
		}
	})

	t.Run("SyntaxError", func(t *testing.T) {
		_, err := rt.Eval("[")
		if err == nil {
			t.Fatal("expected error from Eval")
		}
		var pyErr *pyffi.PythonError
		if !errors.As(err, &pyErr) {
			t.Fatalf("expected *PythonError, got %T: %v", err, err)
		}
		if pyErr.Type != "SyntaxError" {
			t.Errorf("Type = %q, want %q", pyErr.Type, "SyntaxError")
		}
	})

	t.Run("NameError", func(t *testing.T) {
		_, err := rt.Eval("undefined_variable_xyz")
		if err == nil {
			t.Fatal("expected error from Eval")
		}
		var pyErr *pyffi.PythonError
		if !errors.As(err, &pyErr) {
			t.Fatalf("expected *PythonError, got %T: %v", err, err)
		}
		if !strings.Contains(pyErr.Type, "NameError") {
			t.Errorf("Type = %q, want NameError", pyErr.Type)
		}
	})

	t.Run("None", func(t *testing.T) {
		result, err := rt.Eval("None")
		if err != nil {
			t.Fatalf("Eval failed: %v", err)
		}
		defer result.Close()

		val, err := result.GoValue()
		if err != nil {
			t.Fatalf("GoValue failed: %v", err)
		}
		if val != nil {
			t.Errorf("Eval(None) = %v, want nil", val)
		}
	})
}
