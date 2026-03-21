package pyffi_test

import (
	"os"
	"testing"

	"github.com/i2y/pyffi"
)

// sharedRT is a process-global Runtime shared across all tests.
// This avoids Py_Initialize/Py_Finalize cycles which cause SIGSEGV.
// With auto-GIL, all public methods automatically acquire the GIL,
// so tests can call them from any goroutine without manual GIL management.
var sharedRT *pyffi.Runtime

func TestMain(m *testing.M) {
	rt, err := pyffi.New()
	if err != nil {
		os.Exit(m.Run())
	}
	sharedRT = rt
	code := m.Run()
	// Skip Close() — Py_Finalize can hang on some Linux environments.
	// Process exit handles cleanup.
	os.Exit(code)
}

func newOrSkip(t *testing.T) *pyffi.Runtime {
	t.Helper()
	if sharedRT == nil {
		t.Skip("Python not available")
	}
	return sharedRT
}
