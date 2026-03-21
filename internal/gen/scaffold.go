package gen

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/i2y/pyffi"
)

// ScaffoldInit generates pyproject.toml and pyffi-gen.yaml.
func ScaffoldInit(modules []string, pythonConstraint string) error {
	if !pyffi.UVAvailable() {
		return fmt.Errorf("pyffi-gen: uv not found; install: curl -LsSf https://astral.sh/uv/install.sh | sh")
	}

	if pythonConstraint == "" {
		pythonConstraint = ">=3.11"
	}

	// Generate pyproject.toml.
	pyproject := fmt.Sprintf(`[project]
name = "pyffi-project"
version = "0.1.0"
requires-python = "%s"
dependencies = [
`, pythonConstraint)
	for _, m := range modules {
		pyproject += fmt.Sprintf("    %q,\n", m)
	}
	pyproject += `]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`

	if err := os.WriteFile("pyproject.toml", []byte(pyproject), 0644); err != nil {
		return fmt.Errorf("pyffi-gen: write pyproject.toml: %w", err)
	}
	fmt.Println("wrote pyproject.toml")

	// Generate pyffi-gen.yaml (first module only for single-module config).
	if len(modules) > 0 {
		m := modules[0]
		pkg := strings.ReplaceAll(m, ".", "_")
		yaml := fmt.Sprintf(`module: %s
package: %s
out: ./gen/%s
test: true
`, m, pkg, pkg)

		if err := os.WriteFile("pyffi-gen.yaml", []byte(yaml), 0644); err != nil {
			return fmt.Errorf("pyffi-gen: write pyffi-gen.yaml: %w", err)
		}
		fmt.Println("wrote pyffi-gen.yaml")
	}

	// Run uv sync.
	fmt.Println("running uv sync...")
	return ScaffoldSync()
}

// ScaffoldSync runs uv sync in the current directory.
func ScaffoldSync() error {
	if !pyffi.UVAvailable() {
		return fmt.Errorf("pyffi-gen: uv not found")
	}
	cmd := exec.Command("uv", "sync")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
