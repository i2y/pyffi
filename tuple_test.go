package pyffi_test

import (
	"testing"
)

func TestTupleSupport(t *testing.T) {
	rt := newOrSkip(t)

	t.Run("TupleConversion", func(t *testing.T) {
		// Python tuple → Go []any via GoValue
		if err := rt.Exec(`t = (1, "two", 3.0, True)`); err != nil {
			t.Fatalf("Exec failed: %v", err)
		}
		mod, err := rt.Import("__main__")
		if err != nil {
			t.Fatalf("Import failed: %v", err)
		}
		defer mod.Close()

		obj := mod.Attr("t")
		if obj == nil {
			t.Fatal("expected t attribute")
		}
		defer obj.Close()

		val, err := obj.GoValue()
		if err != nil {
			t.Fatalf("GoValue failed: %v", err)
		}
		items, ok := val.([]any)
		if !ok {
			t.Fatalf("GoValue returned %T, want []any", val)
		}
		if len(items) != 4 {
			t.Fatalf("len = %d, want 4", len(items))
		}
		if items[0] != int64(1) {
			t.Errorf("items[0] = %v, want int64(1)", items[0])
		}
		if items[1] != "two" {
			t.Errorf("items[1] = %v, want %q", items[1], "two")
		}
	})

	t.Run("NewTuple", func(t *testing.T) {
		tuple, err := rt.NewTuple(10, "hello", true)
		if err != nil {
			t.Fatalf("NewTuple failed: %v", err)
		}
		defer tuple.Close()

		val, err := tuple.GoValue()
		if err != nil {
			t.Fatalf("GoValue failed: %v", err)
		}
		items := val.([]any)
		if len(items) != 3 {
			t.Fatalf("len = %d, want 3", len(items))
		}
		if items[0] != int64(10) {
			t.Errorf("items[0] = %v, want 10", items[0])
		}
		if items[2] != true {
			t.Errorf("items[2] = %v, want true", items[2])
		}
	})

	t.Run("EmptyTuple", func(t *testing.T) {
		tuple, err := rt.NewTuple()
		if err != nil {
			t.Fatalf("NewTuple failed: %v", err)
		}
		defer tuple.Close()

		val, err := tuple.GoValue()
		if err != nil {
			t.Fatalf("GoValue failed: %v", err)
		}
		items := val.([]any)
		if len(items) != 0 {
			t.Errorf("expected empty tuple, got %v", items)
		}
	})

	t.Run("TupleAsCallArg", func(t *testing.T) {
		// Pass tuple to a Python function
		if err := rt.Exec(`
def sum_tuple(t):
    return sum(t)
`); err != nil {
			t.Fatalf("Exec failed: %v", err)
		}
		mod, err := rt.Import("__main__")
		if err != nil {
			t.Fatalf("Import failed: %v", err)
		}
		defer mod.Close()

		fn := mod.Attr("sum_tuple")
		if fn == nil {
			t.Fatal("expected sum_tuple")
		}
		defer fn.Close()

		tuple, err := rt.NewTuple(1, 2, 3)
		if err != nil {
			t.Fatalf("NewTuple failed: %v", err)
		}
		defer tuple.Close()

		result, err := fn.Call(tuple)
		if err != nil {
			t.Fatalf("Call failed: %v", err)
		}
		defer result.Close()

		val, err := result.Int64()
		if err != nil {
			t.Fatalf("Int64 failed: %v", err)
		}
		if val != 6 {
			t.Errorf("sum = %d, want 6", val)
		}
	})

	t.Run("TupleFromFunction", func(t *testing.T) {
		// Functions returning tuples should auto-convert
		result, err := rt.Eval("divmod(10, 3)")
		if err != nil {
			t.Fatalf("Eval failed: %v", err)
		}
		defer result.Close()

		val, err := result.GoValue()
		if err != nil {
			t.Fatalf("GoValue failed: %v", err)
		}
		items := val.([]any)
		if len(items) != 2 {
			t.Fatalf("len = %d, want 2", len(items))
		}
		if items[0] != int64(3) || items[1] != int64(1) {
			t.Errorf("divmod(10,3) = %v, want [3, 1]", items)
		}
	})
}
