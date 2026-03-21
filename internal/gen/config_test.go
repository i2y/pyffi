package gen

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pyffi-gen.yaml")
	content := `
module: json
package: jsonpkg
out: ./gen/jsonpkg
include:
  - dumps
  - loads
exclude:
  - "_*"
type_overrides:
  "numpy.ndarray": "[][]float64"
test: true
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Module != "json" {
		t.Errorf("Module = %q, want json", cfg.Module)
	}
	if cfg.Package != "jsonpkg" {
		t.Errorf("Package = %q, want jsonpkg", cfg.Package)
	}
	if cfg.Out != "./gen/jsonpkg" {
		t.Errorf("Out = %q", cfg.Out)
	}
	if len(cfg.Include) != 2 {
		t.Errorf("Include len = %d, want 2", len(cfg.Include))
	}
	if len(cfg.Exclude) != 1 {
		t.Errorf("Exclude len = %d, want 1", len(cfg.Exclude))
	}
	if cfg.TypeOverrides["numpy.ndarray"] != "[][]float64" {
		t.Errorf("TypeOverrides missing")
	}
	if !cfg.Test {
		t.Error("Test should be true")
	}
}

func TestConfigDefaults(t *testing.T) {
	cfg := &Config{Module: "sklearn.ensemble"}
	cfg.Defaults()

	if cfg.Package != "sklearn_ensemble" {
		t.Errorf("Package = %q, want sklearn_ensemble", cfg.Package)
	}
	if cfg.Out != "./gen/sklearn_ensemble" {
		t.Errorf("Out = %q", cfg.Out)
	}
	if cfg.DocStyle != "godoc" {
		t.Errorf("DocStyle = %q, want godoc", cfg.DocStyle)
	}
}

func TestConfigValidate(t *testing.T) {
	cfg := &Config{}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for empty module")
	}

	cfg.Module = "json"
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
