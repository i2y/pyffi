package pyffi_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/i2y/pyffi"
)

// TestErrorHandling consolidates all error-related tests into a single Runtime
// to minimize Py_Initialize/Py_Finalize cycles (known CPython instability).
func TestErrorHandling(t *testing.T) {
	rt := newOrSkip(t)

	t.Run("ExecErrorHasTypeAndMessage", func(t *testing.T) {
		err := rt.Exec(`raise ValueError("test")`)
		if err == nil {
			t.Fatal("expected error from Exec")
		}
		var pyErr *pyffi.PythonError
		if !errors.As(err, &pyErr) {
			t.Fatalf("expected *PythonError, got %T: %v", err, err)
		}
		if pyErr.Type != "ValueError" {
			t.Errorf("Type = %q, want %q", pyErr.Type, "ValueError")
		}
		if pyErr.Message != "test" {
			t.Errorf("Message = %q, want %q", pyErr.Message, "test")
		}
	})

	t.Run("ErrorTraceback", func(t *testing.T) {
		err := rt.Exec(`
def foo():
    raise ValueError("test error")
foo()
`)
		if err == nil {
			t.Fatal("expected error from Exec")
		}
		var pyErr *pyffi.PythonError
		if !errors.As(err, &pyErr) {
			t.Fatalf("expected *PythonError, got %T: %v", err, err)
		}
		if pyErr.Type != "ValueError" {
			t.Errorf("Type = %q, want %q", pyErr.Type, "ValueError")
		}
		if pyErr.Message != "test error" {
			t.Errorf("Message = %q, want %q", pyErr.Message, "test error")
		}
		if pyErr.Traceback == "" {
			t.Fatal("expected non-empty Traceback")
		}
		if !strings.Contains(pyErr.Traceback, "foo") {
			t.Errorf("Traceback should contain function name 'foo':\n%s", pyErr.Traceback)
		}
		if !strings.Contains(pyErr.Traceback, "Traceback") {
			t.Errorf("Traceback should contain 'Traceback':\n%s", pyErr.Traceback)
		}
	})

	t.Run("NestedErrorTraceback", func(t *testing.T) {
		err := rt.Exec(`
def a():
    return b()
def b():
    return c()
def c():
    raise RuntimeError("deep error")
a()
`)
		if err == nil {
			t.Fatal("expected error from Exec")
		}
		var pyErr *pyffi.PythonError
		if !errors.As(err, &pyErr) {
			t.Fatalf("expected *PythonError, got %T: %v", err, err)
		}
		if pyErr.Type != "RuntimeError" {
			t.Errorf("Type = %q, want %q", pyErr.Type, "RuntimeError")
		}
		if pyErr.Message != "deep error" {
			t.Errorf("Message = %q, want %q", pyErr.Message, "deep error")
		}
		if pyErr.Traceback == "" {
			t.Fatal("expected non-empty Traceback")
		}
		for _, name := range []string{"a", "b", "c"} {
			if !strings.Contains(pyErr.Traceback, name) {
				t.Errorf("Traceback should contain function name %q:\n%s", name, pyErr.Traceback)
			}
		}
	})

	t.Run("SyntaxError", func(t *testing.T) {
		err := rt.Exec(`def`)
		if err == nil {
			t.Fatal("expected error from Exec")
		}
		var pyErr *pyffi.PythonError
		if !errors.As(err, &pyErr) {
			t.Fatalf("expected *PythonError, got %T: %v", err, err)
		}
		if pyErr.Type != "SyntaxError" {
			t.Errorf("Type = %q, want %q", pyErr.Type, "SyntaxError")
		}
	})

	t.Run("CallErrorTraceback", func(t *testing.T) {
		// Define a function that raises.
		if err := rt.Exec(`
def failing_func():
    raise TypeError("bad type")
`); err != nil {
			t.Fatalf("setup Exec failed: %v", err)
		}

		mod, err := rt.Import("__main__")
		if err != nil {
			t.Fatalf("Import __main__ failed: %v", err)
		}
		defer mod.Close()

		fn := mod.Attr("failing_func")
		if fn == nil {
			t.Fatal("expected failing_func attribute")
		}
		defer fn.Close()

		_, err = fn.Call()
		if err == nil {
			t.Fatal("expected error from Call")
		}
		var pyErr *pyffi.PythonError
		if !errors.As(err, &pyErr) {
			t.Fatalf("expected *PythonError, got %T: %v", err, err)
		}
		if pyErr.Type != "TypeError" {
			t.Errorf("Type = %q, want %q", pyErr.Type, "TypeError")
		}
		if pyErr.Message != "bad type" {
			t.Errorf("Message = %q, want %q", pyErr.Message, "bad type")
		}
		if pyErr.Traceback == "" {
			t.Fatal("expected non-empty Traceback")
		}
		if !strings.Contains(pyErr.Traceback, "failing_func") {
			t.Errorf("Traceback should contain 'failing_func':\n%s", pyErr.Traceback)
		}
	})
}
