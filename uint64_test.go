package pyffi_test

import (
	"math"
	"testing"
)

func TestUint64Conversion(t *testing.T) {
	rt := newOrSkip(t)

	t.Run("Uint64Max", func(t *testing.T) {
		obj, err := rt.FromGoValue(uint64(math.MaxUint64))
		if err != nil {
			t.Fatalf("FromGoValue failed: %v", err)
		}
		defer obj.Close()

		// Verify via Python that the value is correct
		if err := rt.Exec(`
def check_uint64(v):
    return v == 18446744073709551615
`); err != nil {
			t.Fatalf("Exec failed: %v", err)
		}
		mod, _ := rt.Import("__main__")
		defer mod.Close()
		fn := mod.Attr("check_uint64")
		defer fn.Close()

		result, err := fn.Call(obj)
		if err != nil {
			t.Fatalf("Call failed: %v", err)
		}
		defer result.Close()
		b, _ := result.Bool()
		if !b {
			t.Error("uint64 max value not preserved")
		}
	})

	t.Run("Uint64OverInt64Max", func(t *testing.T) {
		val := uint64(math.MaxInt64) + 1 // 9223372036854775808
		obj, err := rt.FromGoValue(val)
		if err != nil {
			t.Fatalf("FromGoValue failed: %v", err)
		}
		defer obj.Close()

		if err := rt.Exec(`
def check_over_int64(v):
    return v == 9223372036854775808
`); err != nil {
			t.Fatalf("Exec failed: %v", err)
		}
		mod, _ := rt.Import("__main__")
		defer mod.Close()
		fn := mod.Attr("check_over_int64")
		defer fn.Close()

		result, err := fn.Call(obj)
		if err != nil {
			t.Fatalf("Call failed: %v", err)
		}
		defer result.Close()
		b, _ := result.Bool()
		if !b {
			t.Error("uint64 value over int64 max not preserved")
		}
	})

	t.Run("UintLargeValue", func(t *testing.T) {
		// uint on 64-bit should also work for large values
		val := uint(math.MaxInt64) + 1
		obj, err := rt.FromGoValue(val)
		if err != nil {
			t.Fatalf("FromGoValue failed: %v", err)
		}
		defer obj.Close()

		if err := rt.Exec(`
def check_large_uint(v):
    return v == 9223372036854775808
`); err != nil {
			t.Fatalf("Exec failed: %v", err)
		}
		mod, _ := rt.Import("__main__")
		defer mod.Close()
		fn := mod.Attr("check_large_uint")
		defer fn.Close()

		result, err := fn.Call(obj)
		if err != nil {
			t.Fatalf("Call failed: %v", err)
		}
		defer result.Close()
		b, _ := result.Bool()
		if !b {
			t.Error("large uint value not preserved")
		}
	})

	t.Run("Uint64Zero", func(t *testing.T) {
		obj, err := rt.FromGoValue(uint64(0))
		if err != nil {
			t.Fatalf("FromGoValue failed: %v", err)
		}
		defer obj.Close()

		val, err := obj.Int64()
		if err != nil {
			t.Fatalf("Int64 failed: %v", err)
		}
		if val != 0 {
			t.Errorf("val = %d, want 0", val)
		}
	})
}
