package pyffi_test

import (
	"errors"
	"testing"

	"github.com/i2y/pyffi"
)

func TestNewAndClose(t *testing.T) {
	rt := newOrSkip(t)
	if err := rt.Exec("pass"); err != nil {
		t.Fatalf("Exec failed: %v", err)
	}
}

func TestExecSuccess(t *testing.T) {
	rt := newOrSkip(t)
	if err := rt.Exec("x = 1 + 1"); err != nil {
		t.Fatalf("Exec failed: %v", err)
	}
}

func TestExecPrint(t *testing.T) {
	rt := newOrSkip(t)
	if err := rt.Exec(`print("hello from python")`); err != nil {
		t.Fatalf("Exec failed: %v", err)
	}
}

func TestExecError(t *testing.T) {
	rt := newOrSkip(t)
	err := rt.Exec(`raise ValueError("test error")`)
	if err == nil {
		t.Fatal("expected error from Exec")
	}
	var pyErr *pyffi.PythonError
	if !errors.As(err, &pyErr) {
		t.Fatalf("expected *PythonError, got %T: %v", err, err)
	}
}

func TestImportModule(t *testing.T) {
	rt := newOrSkip(t)
	obj, err := rt.ImportModule("sys")
	if err != nil {
		t.Fatalf("ImportModule failed: %v", err)
	}
	if obj.IsNull() {
		t.Fatal("expected non-null PyObject")
	}
}

func TestImportModuleNotFound(t *testing.T) {
	rt := newOrSkip(t)
	_, err := rt.ImportModule("nonexistent_module_xyz_12345")
	if err == nil {
		t.Fatal("expected error from ImportModule")
	}
	var pyErr *pyffi.PythonError
	if !errors.As(err, &pyErr) {
		t.Fatalf("expected *PythonError, got %T: %v", err, err)
	}
}
