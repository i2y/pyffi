package pyffi_test

import (
	"sort"
	"testing"
)

func TestSetSupport(t *testing.T) {
	rt := newOrSkip(t)

	t.Run("SetConversion", func(t *testing.T) {
		if err := rt.Exec(`s = {3, 1, 2}`); err != nil {
			t.Fatalf("Exec failed: %v", err)
		}
		mod, err := rt.Import("__main__")
		if err != nil {
			t.Fatalf("Import failed: %v", err)
		}
		defer mod.Close()

		obj := mod.Attr("s")
		if obj == nil {
			t.Fatal("expected s attribute")
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
		if len(items) != 3 {
			t.Fatalf("len = %d, want 3", len(items))
		}
		// Sets are unordered, so sort for comparison
		ints := make([]int, len(items))
		for i, v := range items {
			ints[i] = int(v.(int64))
		}
		sort.Ints(ints)
		if ints[0] != 1 || ints[1] != 2 || ints[2] != 3 {
			t.Errorf("set values = %v, want [1, 2, 3]", ints)
		}
	})

	t.Run("FrozenSetConversion", func(t *testing.T) {
		if err := rt.Exec(`fs = frozenset({10, 20})`); err != nil {
			t.Fatalf("Exec failed: %v", err)
		}
		mod, err := rt.Import("__main__")
		if err != nil {
			t.Fatalf("Import failed: %v", err)
		}
		defer mod.Close()

		obj := mod.Attr("fs")
		if obj == nil {
			t.Fatal("expected fs attribute")
		}
		defer obj.Close()

		val, err := obj.GoValue()
		if err != nil {
			t.Fatalf("GoValue failed: %v", err)
		}
		items := val.([]any)
		if len(items) != 2 {
			t.Fatalf("len = %d, want 2", len(items))
		}
	})

	t.Run("NewSet", func(t *testing.T) {
		set, err := rt.NewSet(1, 2, 3, 2, 1)
		if err != nil {
			t.Fatalf("NewSet failed: %v", err)
		}
		defer set.Close()

		n, err := set.Len()
		if err != nil {
			t.Fatalf("Len failed: %v", err)
		}
		if n != 3 {
			t.Errorf("Len = %d, want 3 (duplicates removed)", n)
		}
	})

	t.Run("EmptySet", func(t *testing.T) {
		set, err := rt.NewSet()
		if err != nil {
			t.Fatalf("NewSet failed: %v", err)
		}
		defer set.Close()

		n, err := set.Len()
		if err != nil {
			t.Fatalf("Len failed: %v", err)
		}
		if n != 0 {
			t.Errorf("Len = %d, want 0", n)
		}
	})

	t.Run("Iterator", func(t *testing.T) {
		list, err := rt.NewList("a", "b", "c")
		if err != nil {
			t.Fatalf("NewList failed: %v", err)
		}
		defer list.Close()

		iter, err := list.Iter()
		if err != nil {
			t.Fatalf("Iter failed: %v", err)
		}
		defer iter.Close()

		var items []string
		for {
			item, err := iter.Next()
			if err != nil {
				t.Fatalf("Next failed: %v", err)
			}
			if item == nil {
				break
			}
			s, _ := item.GoString()
			items = append(items, s)
			item.Close()
		}
		if len(items) != 3 {
			t.Fatalf("len = %d, want 3", len(items))
		}
		if items[0] != "a" || items[1] != "b" || items[2] != "c" {
			t.Errorf("items = %v, want [a, b, c]", items)
		}
	})
}
