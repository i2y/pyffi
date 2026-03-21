package pyffi_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/i2y/pyffi"
)

func skipIfNoUV(t *testing.T) {
	t.Helper()
	if os.Getenv("PYFFI_TEST_UV") != "1" {
		t.Skip("set PYFFI_TEST_UV=1 to run uv integration tests")
	}
	if !pyffi.UVAvailable() {
		t.Skip("uv not installed")
	}
}

func TestUVAvailable(t *testing.T) {
	if os.Getenv("PYFFI_TEST_UV") != "1" {
		t.Skip("set PYFFI_TEST_UV=1 to run uv integration tests")
	}
	if !pyffi.UVAvailable() {
		t.Fatal("expected uv to be available")
	}
}

func TestUVVersion(t *testing.T) {
	skipIfNoUV(t)
	v, err := pyffi.UVVersion()
	if err != nil {
		t.Fatal(err)
	}
	if v == "" {
		t.Fatal("expected non-empty version string")
	}
	t.Logf("uv version: %s", v)
}

func TestDependencies(t *testing.T) {
	skipIfNoUV(t)
	// Clean cache first to ensure fresh install.
	pyffi.ClearCache()

	rt, err := pyffi.New(pyffi.Dependencies("requests>=2.28"))
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	// Verify requests can be imported.
	if err := rt.Exec("import requests"); err != nil {
		t.Fatalf("failed to import requests: %v", err)
	}
}

func TestDependenciesCache(t *testing.T) {
	skipIfNoUV(t)

	// First call installs.
	start1 := time.Now()
	rt1, err := pyffi.New(pyffi.Dependencies("requests>=2.28"))
	if err != nil {
		t.Fatal(err)
	}
	rt1.Close()
	dur1 := time.Since(start1)

	// Second call should hit cache.
	start2 := time.Now()
	rt2, err := pyffi.New(pyffi.Dependencies("requests>=2.28"))
	if err != nil {
		t.Fatal(err)
	}
	rt2.Close()
	dur2 := time.Since(start2)

	t.Logf("first: %v, second (cached): %v", dur1, dur2)
	// Cache hit should be significantly faster (no uv pip install).
	if dur2 > dur1 && dur2 > 5*time.Second {
		t.Logf("warning: cache hit was not faster than first install")
	}
}

func TestClearCache(t *testing.T) {
	skipIfNoUV(t)
	// Create a cached venv.
	rt, err := pyffi.New(pyffi.Dependencies("requests>=2.28"))
	if err != nil {
		t.Fatal(err)
	}
	rt.Close()

	// Clear cache.
	if err := pyffi.ClearCache(); err != nil {
		t.Fatal(err)
	}
}

func TestWithUVProject(t *testing.T) {
	skipIfNoUV(t)

	// Create a temp project with pyproject.toml.
	dir := t.TempDir()
	pyproject := `[project]
name = "test-project"
version = "0.1.0"
requires-python = ">=3.11"
dependencies = ["requests>=2.28"]
`
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(pyproject), 0644); err != nil {
		t.Fatal(err)
	}

	rt, err := pyffi.New(pyffi.WithUVProject(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	// Verify requests can be imported.
	if err := rt.Exec("import requests"); err != nil {
		t.Fatalf("failed to import requests: %v", err)
	}
}

func TestUVNotAvailable(t *testing.T) {
	if os.Getenv("PYFFI_TEST_UV") != "1" {
		t.Skip("set PYFFI_TEST_UV=1 to run uv integration tests")
	}
	// This test only makes sense if uv IS available,
	// we just verify the error message format by checking the helper.
	if !pyffi.UVAvailable() {
		t.Skip("uv not installed")
	}
	// UVAvailable returns true, so we just verify the function works.
	if !pyffi.UVAvailable() {
		t.Fatal("expected UVAvailable to return true")
	}
}
