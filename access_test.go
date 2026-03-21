package pyffi_test

import (
	"testing"
)

func TestObjectAccess(t *testing.T) {
	rt := newOrSkip(t)

	t.Run("GetItemDict", func(t *testing.T) {
		dict, err := rt.NewDict("a", 1, "b", 2)
		if err != nil {
			t.Fatalf("NewDict failed: %v", err)
		}
		defer dict.Close()

		val, err := dict.GetItem("a")
		if err != nil {
			t.Fatalf("GetItem failed: %v", err)
		}
		defer val.Close()

		n, err := val.Int64()
		if err != nil {
			t.Fatalf("Int64 failed: %v", err)
		}
		if n != 1 {
			t.Errorf("dict['a'] = %d, want 1", n)
		}
	})

	t.Run("GetItemList", func(t *testing.T) {
		list, err := rt.NewList("x", "y", "z")
		if err != nil {
			t.Fatalf("NewList failed: %v", err)
		}
		defer list.Close()

		val, err := list.GetItem(1)
		if err != nil {
			t.Fatalf("GetItem failed: %v", err)
		}
		defer val.Close()

		s, err := val.GoString()
		if err != nil {
			t.Fatalf("GoString failed: %v", err)
		}
		if s != "y" {
			t.Errorf("list[1] = %q, want %q", s, "y")
		}
	})

	t.Run("GetItemKeyError", func(t *testing.T) {
		dict, err := rt.NewDict("a", 1)
		if err != nil {
			t.Fatalf("NewDict failed: %v", err)
		}
		defer dict.Close()

		_, err = dict.GetItem("missing")
		if err == nil {
			t.Fatal("expected error for missing key")
		}
	})

	t.Run("LenList", func(t *testing.T) {
		list, err := rt.NewList(1, 2, 3, 4, 5)
		if err != nil {
			t.Fatalf("NewList failed: %v", err)
		}
		defer list.Close()

		n, err := list.Len()
		if err != nil {
			t.Fatalf("Len failed: %v", err)
		}
		if n != 5 {
			t.Errorf("Len = %d, want 5", n)
		}
	})

	t.Run("LenDict", func(t *testing.T) {
		dict, err := rt.NewDict("a", 1, "b", 2)
		if err != nil {
			t.Fatalf("NewDict failed: %v", err)
		}
		defer dict.Close()

		n, err := dict.Len()
		if err != nil {
			t.Fatalf("Len failed: %v", err)
		}
		if n != 2 {
			t.Errorf("Len = %d, want 2", n)
		}
	})

	t.Run("LenString", func(t *testing.T) {
		obj := rt.FromString("hello")
		defer obj.Close()

		n, err := obj.Len()
		if err != nil {
			t.Fatalf("Len failed: %v", err)
		}
		if n != 5 {
			t.Errorf("Len = %d, want 5", n)
		}
	})

	t.Run("Repr", func(t *testing.T) {
		list, err := rt.NewList(1, 2, 3)
		if err != nil {
			t.Fatalf("NewList failed: %v", err)
		}
		defer list.Close()

		r, err := list.Repr()
		if err != nil {
			t.Fatalf("Repr failed: %v", err)
		}
		if r != "[1, 2, 3]" {
			t.Errorf("Repr = %q, want %q", r, "[1, 2, 3]")
		}
	})

	t.Run("ReprString", func(t *testing.T) {
		obj := rt.FromString("hi")
		defer obj.Close()

		r, err := obj.Repr()
		if err != nil {
			t.Fatalf("Repr failed: %v", err)
		}
		if r != "'hi'" {
			t.Errorf("Repr = %q, want %q", r, "'hi'")
		}
	})

	t.Run("DelItemDict", func(t *testing.T) {
		dict, err := rt.NewDict("a", 1, "b", 2, "c", 3)
		if err != nil {
			t.Fatalf("NewDict failed: %v", err)
		}
		defer dict.Close()

		if err := dict.DelItem("b"); err != nil {
			t.Fatalf("DelItem failed: %v", err)
		}
		n, _ := dict.Len()
		if n != 2 {
			t.Errorf("Len after delete = %d, want 2", n)
		}
		// Verify "b" is gone
		_, err = dict.GetItem("b")
		if err == nil {
			t.Error("expected error getting deleted key")
		}
	})

	t.Run("DelItemList", func(t *testing.T) {
		list, err := rt.NewList("x", "y", "z")
		if err != nil {
			t.Fatalf("NewList failed: %v", err)
		}
		defer list.Close()

		if err := list.DelItem(1); err != nil {
			t.Fatalf("DelItem failed: %v", err)
		}
		n, _ := list.Len()
		if n != 2 {
			t.Errorf("Len after delete = %d, want 2", n)
		}
	})

	t.Run("ContainsList", func(t *testing.T) {
		list, err := rt.NewList(1, 2, 3)
		if err != nil {
			t.Fatalf("NewList failed: %v", err)
		}
		defer list.Close()

		ok, err := list.Contains(2)
		if err != nil {
			t.Fatalf("Contains failed: %v", err)
		}
		if !ok {
			t.Error("Contains(2) = false, want true")
		}

		ok, err = list.Contains(99)
		if err != nil {
			t.Fatalf("Contains failed: %v", err)
		}
		if ok {
			t.Error("Contains(99) = true, want false")
		}
	})

	t.Run("ContainsString", func(t *testing.T) {
		s := rt.FromString("hello world")
		defer s.Close()

		ok, err := s.Contains("world")
		if err != nil {
			t.Fatalf("Contains failed: %v", err)
		}
		if !ok {
			t.Error("Contains('world') = false, want true")
		}

		ok, err = s.Contains("xyz")
		if err != nil {
			t.Fatalf("Contains failed: %v", err)
		}
		if ok {
			t.Error("Contains('xyz') = true, want false")
		}
	})

	t.Run("ContainsSet", func(t *testing.T) {
		set, err := rt.NewSet(10, 20, 30)
		if err != nil {
			t.Fatalf("NewSet failed: %v", err)
		}
		defer set.Close()

		ok, err := set.Contains(20)
		if err != nil {
			t.Fatalf("Contains failed: %v", err)
		}
		if !ok {
			t.Error("Contains(20) = false, want true")
		}
	})
}
