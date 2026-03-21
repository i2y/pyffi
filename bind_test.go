package pyffi_test

import (
	"testing"

	"github.com/i2y/pyffi"
)

func TestBind(t *testing.T) {
	rt := newOrSkip(t)

	m, err := pyffi.Bind(rt, "json")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
}

func TestBindCall(t *testing.T) {
	rt := newOrSkip(t)

	m, err := pyffi.Bind(rt, "json")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	data, err := rt.FromGoValue(map[string]any{"key": "value"})
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()

	result, err := m.Call("dumps", data)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()

	s, err := result.GoString()
	if err != nil {
		t.Fatal(err)
	}
	if s != `{"key": "value"}` {
		t.Fatalf("got %q", s)
	}
}

func TestBindCallKw(t *testing.T) {
	rt := newOrSkip(t)

	m, err := pyffi.Bind(rt, "json")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	data, err := rt.FromGoValue(map[string]any{"a": int64(1)})
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()

	result, err := m.CallKw("dumps", []any{data}, map[string]any{"sort_keys": true})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()

	s, err := result.GoString()
	if err != nil {
		t.Fatal(err)
	}
	if s != `{"a": 1}` {
		t.Fatalf("got %q", s)
	}
}

func TestBindNames(t *testing.T) {
	rt := newOrSkip(t)

	m, err := pyffi.Bind(rt, "json")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	names, err := m.Names()
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, n := range names {
		if n == "dumps" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'dumps' in Names()")
	}
}

func TestBindHas(t *testing.T) {
	rt := newOrSkip(t)

	m, err := pyffi.Bind(rt, "json")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	if !m.Has("dumps") {
		t.Fatal("expected Has(dumps) to be true")
	}
	if m.Has("nonexistent_xyz") {
		t.Fatal("expected Has(nonexistent_xyz) to be false")
	}
}

func TestBindNotFound(t *testing.T) {
	rt := newOrSkip(t)

	_, err := pyffi.Bind(rt, "nonexistent_module_xyz_99999")
	if err == nil {
		t.Fatal("expected error for nonexistent module")
	}
}
