package pyffi_test

import (
	"fmt"
	"testing"

	"github.com/i2y/pyffi"
)

func TestContextManager(t *testing.T) {
	rt := newOrSkip(t)

	t.Run("With", func(t *testing.T) {
		// Define a simple context manager in Python
		if err := rt.Exec(`
class CM:
    def __init__(self):
        self.entered = False
        self.exited = False
    def __enter__(self):
        self.entered = True
        return self
    def __exit__(self, *args):
        self.exited = True
        return False
cm = CM()
`); err != nil {
			t.Fatalf("Exec failed: %v", err)
		}

		mod, err := rt.Import("__main__")
		if err != nil {
			t.Fatalf("Import failed: %v", err)
		}
		defer mod.Close()

		cm := mod.Attr("cm")
		if cm == nil {
			t.Fatal("expected cm attribute")
		}
		defer cm.Close()

		var calledInside bool
		err = rt.With(cm, func(value *pyffi.Object) error {
			calledInside = true
			// Check that __enter__ was called
			entered := value.Attr("entered")
			if entered == nil {
				t.Fatal("expected entered attribute")
			}
			defer entered.Close()
			b, _ := entered.Bool()
			if !b {
				t.Error("entered should be true inside With")
			}
			return nil
		})
		if err != nil {
			t.Fatalf("With failed: %v", err)
		}
		if !calledInside {
			t.Error("fn was not called")
		}

		// Check that __exit__ was called
		exited := cm.Attr("exited")
		if exited == nil {
			t.Fatal("expected exited attribute")
		}
		defer exited.Close()
		b, _ := exited.Bool()
		if !b {
			t.Error("exited should be true after With")
		}
	})

	t.Run("WithError", func(t *testing.T) {
		// __exit__ should be called even if fn returns error
		if err := rt.Exec(`
class CM2:
    def __init__(self):
        self.exited = False
    def __enter__(self):
        return self
    def __exit__(self, *args):
        self.exited = True
        return False
cm2 = CM2()
`); err != nil {
			t.Fatalf("Exec failed: %v", err)
		}

		mod, err := rt.Import("__main__")
		if err != nil {
			t.Fatalf("Import failed: %v", err)
		}
		defer mod.Close()

		cm2 := mod.Attr("cm2")
		if cm2 == nil {
			t.Fatal("expected cm2 attribute")
		}
		defer cm2.Close()

		testErr := fmt.Errorf("test error")
		err = rt.With(cm2, func(value *pyffi.Object) error {
			return testErr
		})
		if err != testErr {
			t.Errorf("With returned %v, want testErr", err)
		}

		// __exit__ should still have been called
		exited := cm2.Attr("exited")
		defer exited.Close()
		b, _ := exited.Bool()
		if !b {
			t.Error("exited should be true even on error")
		}
	})

	t.Run("EnterExit", func(t *testing.T) {
		// Test Enter/Exit directly
		if err := rt.Exec(`
class CM3:
    def __init__(self):
        self.value = 42
    def __enter__(self):
        return self
    def __exit__(self, *args):
        return False
cm3 = CM3()
`); err != nil {
			t.Fatalf("Exec failed: %v", err)
		}

		mod, err := rt.Import("__main__")
		if err != nil {
			t.Fatalf("Import failed: %v", err)
		}
		defer mod.Close()

		cm3 := mod.Attr("cm3")
		if cm3 == nil {
			t.Fatal("expected cm3 attribute")
		}
		defer cm3.Close()

		value, err := cm3.Enter()
		if err != nil {
			t.Fatalf("Enter failed: %v", err)
		}
		defer value.Close()

		v := value.Attr("value")
		defer v.Close()
		n, _ := v.Int64()
		if n != 42 {
			t.Errorf("value = %d, want 42", n)
		}

		if err := cm3.Exit(nil); err != nil {
			t.Fatalf("Exit failed: %v", err)
		}
	})
}
