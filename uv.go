package pyffi

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// WithUV configures the Runtime to prefer Python installations managed by uv.
// If uv is not found, it falls back to normal detection without error.
func WithUV() Option {
	return func(c *config) {
		c.preferUV = true
	}
}

// WithUVProject configures the Runtime to use a uv-managed project.
// It reads pyproject.toml from projectDir, runs `uv sync`, and uses
// the resulting .venv's Python with all declared dependencies available.
func WithUVProject(projectDir string) Option {
	return func(c *config) {
		c.projectDir = projectDir
		c.preferUV = true
	}
}

// Dependencies declares Python package dependencies inline.
// A cached venv is created with the specified packages installed.
// Requires uv to be installed.
func Dependencies(deps ...string) Option {
	return func(c *config) {
		c.dependencies = deps
		c.preferUV = true
	}
}

// UVAvailable reports whether the uv command is available on PATH.
func UVAvailable() bool {
	_, err := exec.LookPath("uv")
	return err == nil
}

// UVVersion returns the uv version string (e.g., "0.9.20").
func UVVersion() (string, error) {
	out, err := exec.Command("uv", "self", "version").Output()
	if err != nil {
		return "", fmt.Errorf("pyffi: uv self version: %w", err)
	}
	// Output: "uv 0.9.20 (765a96723 2025-12-29)\n"
	s := strings.TrimSpace(string(out))
	if strings.HasPrefix(s, "uv ") {
		parts := strings.Fields(s)
		if len(parts) >= 2 {
			return parts[1], nil
		}
	}
	return s, nil
}

// uvRun executes a uv subcommand and returns an error including stderr on failure.
func uvRun(args ...string) error {
	cmd := exec.Command("uv", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("uv %s: %w\n%s", args[0], err, stderr.String())
	}
	return nil
}

// setupUVProject ensures the project's venv is up to date via `uv sync`.
// Returns the venv directory path.
func setupUVProject(projectDir string) (string, error) {
	if !UVAvailable() {
		return "", fmt.Errorf("pyffi: uv not found; install: curl -LsSf https://astral.sh/uv/install.sh | sh")
	}

	absDir, err := filepath.Abs(projectDir)
	if err != nil {
		return "", fmt.Errorf("pyffi: invalid project dir: %w", err)
	}

	pyproject := filepath.Join(absDir, "pyproject.toml")
	if _, err := os.Stat(pyproject); err != nil {
		return "", fmt.Errorf("pyffi: pyproject.toml not found in %s", absDir)
	}

	if err := uvRun("sync", "--project", absDir); err != nil {
		return "", fmt.Errorf("pyffi: uv sync failed: %w", err)
	}

	venvDir := filepath.Join(absDir, ".venv")
	if _, err := os.Stat(filepath.Join(venvDir, "pyvenv.cfg")); err != nil {
		return "", fmt.Errorf("pyffi: .venv not created by uv sync in %s", absDir)
	}

	return venvDir, nil
}

// ensureDepsVenv creates or reuses a cached venv with the given dependencies.
func ensureDepsVenv(deps []string, major, minor int) (string, error) {
	if !UVAvailable() {
		return "", fmt.Errorf("pyffi: uv not found; install: curl -LsSf https://astral.sh/uv/install.sh | sh")
	}

	hash := hashDeps(deps, major, minor)
	venvDir := cacheVenvDir(hash)

	// Cache hit.
	if _, err := os.Stat(filepath.Join(venvDir, "pyvenv.cfg")); err == nil {
		return venvDir, nil
	}

	// Create venv.
	args := []string{"venv", venvDir}
	if major > 0 && minor > 0 {
		args = append(args, "--python", fmt.Sprintf("%d.%d", major, minor))
	}
	if err := uvRun(args...); err != nil {
		return "", fmt.Errorf("pyffi: failed to create venv: %w", err)
	}

	// Install packages.
	pythonBin := "python"
	if runtime.GOOS == "windows" {
		pythonBin = filepath.Join(venvDir, "Scripts", "python.exe")
	} else {
		pythonBin = filepath.Join(venvDir, "bin", "python")
	}
	installArgs := append([]string{"pip", "install", "-p", pythonBin}, deps...)
	if err := uvRun(installArgs...); err != nil {
		// Clean up partial venv.
		os.RemoveAll(venvDir)
		return "", fmt.Errorf("pyffi: failed to install dependencies: %w", err)
	}

	return venvDir, nil
}

// venvPythonLibrary derives the libpython path from a venv's pyvenv.cfg.
func venvPythonLibrary(venvDir string) (string, error) {
	cfg, err := parsePyvenvCfg(filepath.Join(venvDir, "pyvenv.cfg"))
	if err != nil {
		return "", fmt.Errorf("pyffi: failed to read pyvenv.cfg: %w", err)
	}

	home := cfg["home"] // e.g., /path/to/bin
	if home == "" {
		return "", fmt.Errorf("pyffi: pyvenv.cfg missing 'home' key")
	}

	// Derive prefix from home (bin directory).
	prefix := filepath.Dir(home)

	// Get version from pyvenv.cfg. The key varies: "version_info" or "version".
	version := cfg["version_info"]
	if version == "" {
		version = cfg["version"]
	}
	major, minor := 3, 0
	if version != "" {
		if m, n, ok := parseVersionString(version); ok {
			major, minor = m, n
		}
	}

	ext := libExtension()
	libName := fmt.Sprintf("libpython%d.%d%s", major, minor, ext)

	// Try prefix/lib/
	p := filepath.Join(prefix, "lib", libName)
	if fileExists(p) {
		return p, nil
	}

	// macOS Framework layout.
	p = filepath.Join(prefix, "Frameworks", "Python.framework", "Versions",
		fmt.Sprintf("%d.%d", major, minor), "lib", libName)
	if fileExists(p) {
		return p, nil
	}

	return "", fmt.Errorf("pyffi: libpython not found for venv at %s (tried prefix %s)", venvDir, prefix)
}

// venvSitePackages returns the site-packages directory inside a venv.
func venvSitePackages(venvDir string) (string, error) {
	pattern := filepath.Join(venvDir, "lib", "python3.*", "site-packages")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("pyffi: site-packages not found in %s", venvDir)
	}
	return matches[0], nil
}

// parsePyvenvCfg reads a pyvenv.cfg file into a key-value map.
func parsePyvenvCfg(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	result := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, "="); idx >= 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			result[key] = val
		}
	}
	return result, scanner.Err()
}

// addSitePackages adds a directory to Python's sys.path.
func (r *Runtime) addSitePackages(siteDir string) {
	code := fmt.Sprintf("import sys; sys.path.insert(0, %q)", siteDir)
	r.pyRunSimpleString(code)
}

// --- Level 1 functions (unchanged) ---

// uvPythonEntry represents an entry from `uv python list --output json`.
type uvPythonEntry struct {
	Key            string `json:"key"`
	Version        string `json:"version"`
	Path           string `json:"path"`
	Symlink        string `json:"symlink"`
	Managed        bool   `json:"managed"`
	DownloadsDir   string `json:"downloads_dir"`
	Implementation string `json:"implementation"`
}

// detectUVCommand runs `uv python list --output json` and returns candidates.
func detectUVCommand() []candidate {
	cmd := exec.Command("uv", "python", "list", "--output", "json")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var entries []uvPythonEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil
	}

	var results []candidate
	for _, e := range entries {
		if e.Implementation != "" && e.Implementation != "cpython" {
			continue
		}
		if e.Path == "" {
			continue
		}
		major, minor, ok := parseVersionString(e.Version)
		if !ok {
			continue
		}
		libPath := deriveLibPathFromExec(e.Path, major, minor)
		if libPath != "" {
			results = append(results, candidate{path: libPath, major: major, minor: minor})
		}
	}
	return results
}

// parseVersionString parses a version string like "3.12.1" into (3, 12, true).
func parseVersionString(v string) (major, minor int, ok bool) {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return 0, 0, false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return major, minor, true
}

// deriveLibPathFromExec derives the shared library path from a Python executable path.
func deriveLibPathFromExec(execPath string, major, minor int) string {
	dir := filepath.Dir(execPath)
	prefix := filepath.Dir(dir)
	ext := libExtension()
	libName := "libpython" + strconv.Itoa(major) + "." + strconv.Itoa(minor) + ext

	p := filepath.Join(prefix, "lib", libName)
	if fileExists(p) {
		return p
	}

	p = filepath.Join(prefix, "Frameworks", "Python.framework", "Versions",
		strconv.Itoa(major)+"."+strconv.Itoa(minor), "lib", libName)
	if fileExists(p) {
		return p
	}

	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
