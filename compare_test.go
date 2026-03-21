package pyffi_test

import (
	"testing"

	"github.com/i2y/pyffi"
)

func TestCompare(t *testing.T) {
	rt := newOrSkip(t)

	t.Run("Equals", func(t *testing.T) {
		a := rt.FromInt64(42)
		defer a.Close()
		b := rt.FromInt64(42)
		defer b.Close()
		c := rt.FromInt64(99)
		defer c.Close()

		eq, err := a.Equals(b)
		if err != nil {
			t.Fatalf("Equals failed: %v", err)
		}
		if !eq {
			t.Error("42 == 42 should be true")
		}

		eq, err = a.Equals(c)
		if err != nil {
			t.Fatalf("Equals failed: %v", err)
		}
		if eq {
			t.Error("42 == 99 should be false")
		}
	})

	t.Run("LessThan", func(t *testing.T) {
		a := rt.FromInt64(1)
		defer a.Close()
		b := rt.FromInt64(2)
		defer b.Close()

		lt, err := a.Compare(b, pyffi.PyLT)
		if err != nil {
			t.Fatalf("Compare failed: %v", err)
		}
		if !lt {
			t.Error("1 < 2 should be true")
		}

		lt, err = b.Compare(a, pyffi.PyLT)
		if err != nil {
			t.Fatalf("Compare failed: %v", err)
		}
		if lt {
			t.Error("2 < 1 should be false")
		}
	})

	t.Run("StringCompare", func(t *testing.T) {
		a := rt.FromString("abc")
		defer a.Close()
		b := rt.FromString("def")
		defer b.Close()

		lt, err := a.Compare(b, pyffi.PyLT)
		if err != nil {
			t.Fatalf("Compare failed: %v", err)
		}
		if !lt {
			t.Error("'abc' < 'def' should be true")
		}
	})

	t.Run("GreaterEqual", func(t *testing.T) {
		a := rt.FromFloat64(3.14)
		defer a.Close()
		b := rt.FromFloat64(2.71)
		defer b.Close()

		ge, err := a.Compare(b, pyffi.PyGE)
		if err != nil {
			t.Fatalf("Compare failed: %v", err)
		}
		if !ge {
			t.Error("3.14 >= 2.71 should be true")
		}
	})

	t.Run("NotEqual", func(t *testing.T) {
		a := rt.FromString("hello")
		defer a.Close()
		b := rt.FromString("world")
		defer b.Close()

		ne, err := a.Compare(b, pyffi.PyNE)
		if err != nil {
			t.Fatalf("Compare failed: %v", err)
		}
		if !ne {
			t.Error("'hello' != 'world' should be true")
		}
	})
}
