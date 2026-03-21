package pyffi_test

import (
	"testing"
)

// TestObjectConstruction consolidates object construction tests into a single Runtime.
func TestObjectConstruction(t *testing.T) {
	rt := newOrSkip(t)

	t.Run("NewList", func(t *testing.T) {
		list, err := rt.NewList(1, "two", 3.0, true)
		if err != nil {
			t.Fatalf("NewList failed: %v", err)
		}
		defer list.Close()

		val, err := list.GoValue()
		if err != nil {
			t.Fatalf("GoValue failed: %v", err)
		}
		items, ok := val.([]any)
		if !ok {
			t.Fatalf("GoValue returned %T, want []any", val)
		}
		if len(items) != 4 {
			t.Fatalf("list length = %d, want 4", len(items))
		}
		if items[0] != int64(1) {
			t.Errorf("items[0] = %v (%T), want int64(1)", items[0], items[0])
		}
		if items[1] != "two" {
			t.Errorf("items[1] = %v, want %q", items[1], "two")
		}
	})

	t.Run("NewListEmpty", func(t *testing.T) {
		list, err := rt.NewList()
		if err != nil {
			t.Fatalf("NewList failed: %v", err)
		}
		defer list.Close()

		val, err := list.GoValue()
		if err != nil {
			t.Fatalf("GoValue failed: %v", err)
		}
		items := val.([]any)
		if len(items) != 0 {
			t.Errorf("expected empty list, got %v", items)
		}
	})

	t.Run("NewDict", func(t *testing.T) {
		dict, err := rt.NewDict("name", "Go", "version", 1.22)
		if err != nil {
			t.Fatalf("NewDict failed: %v", err)
		}
		defer dict.Close()

		val, err := dict.GoValue()
		if err != nil {
			t.Fatalf("GoValue failed: %v", err)
		}
		m, ok := val.(map[string]any)
		if !ok {
			t.Fatalf("GoValue returned %T, want map[string]any", val)
		}
		if m["name"] != "Go" {
			t.Errorf("dict['name'] = %v, want %q", m["name"], "Go")
		}
		if m["version"] != 1.22 {
			t.Errorf("dict['version'] = %v, want 1.22", m["version"])
		}
	})

	t.Run("NewDictEmpty", func(t *testing.T) {
		dict, err := rt.NewDict()
		if err != nil {
			t.Fatalf("NewDict failed: %v", err)
		}
		defer dict.Close()

		val, err := dict.GoValue()
		if err != nil {
			t.Fatalf("GoValue failed: %v", err)
		}
		m := val.(map[string]any)
		if len(m) != 0 {
			t.Errorf("expected empty dict, got %v", m)
		}
	})

	t.Run("NewDictOddArgs", func(t *testing.T) {
		_, err := rt.NewDict("key")
		if err == nil {
			t.Fatal("expected error for odd number of args")
		}
	})

	t.Run("SetAttr", func(t *testing.T) {
		// Create a simple object with settable attributes.
		if err := rt.Exec(`
class Box:
    pass
box = Box()
`); err != nil {
			t.Fatalf("Exec failed: %v", err)
		}

		mod, err := rt.Import("__main__")
		if err != nil {
			t.Fatalf("Import failed: %v", err)
		}
		defer mod.Close()

		box := mod.Attr("box")
		if box == nil {
			t.Fatal("expected box attribute")
		}
		defer box.Close()

		if err := box.SetAttr("value", 42); err != nil {
			t.Fatalf("SetAttr failed: %v", err)
		}

		attr := box.Attr("value")
		if attr == nil {
			t.Fatal("expected value attribute after SetAttr")
		}
		defer attr.Close()

		val, err := attr.Int64()
		if err != nil {
			t.Fatalf("Int64 failed: %v", err)
		}
		if val != 42 {
			t.Errorf("box.value = %d, want 42", val)
		}
	})

	t.Run("SetItem", func(t *testing.T) {
		dict, err := rt.NewDict()
		if err != nil {
			t.Fatalf("NewDict failed: %v", err)
		}
		defer dict.Close()

		if err := dict.SetItem("key", "value"); err != nil {
			t.Fatalf("SetItem failed: %v", err)
		}
		if err := dict.SetItem("num", 99); err != nil {
			t.Fatalf("SetItem failed: %v", err)
		}

		val, err := dict.GoValue()
		if err != nil {
			t.Fatalf("GoValue failed: %v", err)
		}
		m := val.(map[string]any)
		if m["key"] != "value" {
			t.Errorf("dict['key'] = %v, want %q", m["key"], "value")
		}
		if m["num"] != int64(99) {
			t.Errorf("dict['num'] = %v, want 99", m["num"])
		}
	})

	t.Run("SetItemList", func(t *testing.T) {
		list, err := rt.NewList("a", "b", "c")
		if err != nil {
			t.Fatalf("NewList failed: %v", err)
		}
		defer list.Close()

		if err := list.SetItem(1, "B"); err != nil {
			t.Fatalf("SetItem failed: %v", err)
		}

		val, err := list.GoValue()
		if err != nil {
			t.Fatalf("GoValue failed: %v", err)
		}
		items := val.([]any)
		if items[1] != "B" {
			t.Errorf("list[1] = %v, want %q", items[1], "B")
		}
	})
}
