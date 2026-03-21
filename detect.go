package pyffi

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// candidate represents a found Python shared library on disk.
type candidate struct {
	path  string
	major int
	minor int
}

// detectLibrary returns the path to the best matching Python shared library.
func detectLibrary(cfg config) (string, error) {
	if cfg.libraryPath != "" {
		if _, err := os.Stat(cfg.libraryPath); err != nil {
			return "", fmt.Errorf("pyffi: specified library path %q: %w", cfg.libraryPath, err)
		}
		return cfg.libraryPath, nil
	}

	var candidates []candidate
	if cfg.preferUV {
		candidates = append(candidates, detectUVCommand()...)
	}
	candidates = append(candidates, detectCandidates()...)

	if cfg.major > 0 {
		candidates = filterByVersion(candidates, cfg.major, cfg.minor)
	}

	// Deduplicate by resolved path.
	candidates = dedup(candidates)

	// Prefer highest minor version.
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].major != candidates[j].major {
			return candidates[i].major > candidates[j].major
		}
		return candidates[i].minor > candidates[j].minor
	})

	if len(candidates) == 0 {
		return "", &LibraryNotFoundError{Searched: defaultSearchPaths()}
	}

	return candidates[0].path, nil
}

func filterByVersion(cs []candidate, major, minor int) []candidate {
	var out []candidate
	for _, c := range cs {
		if c.major == major && (minor == 0 || c.minor == minor) {
			out = append(out, c)
		}
	}
	return out
}

func dedup(cs []candidate) []candidate {
	seen := make(map[string]bool)
	var out []candidate
	for _, c := range cs {
		resolved, err := filepath.EvalSymlinks(c.path)
		if err != nil {
			resolved = c.path
		}
		if seen[resolved] {
			continue
		}
		seen[resolved] = true
		out = append(out, c)
	}
	return out
}

// parseVersion extracts major.minor from library filenames like
// "libpython3.12.so" or "libpython3.12.dylib".
func parseVersion(filename string) (major, minor int, ok bool) {
	s := strings.TrimPrefix(filename, "libpython")
	if s == filename {
		return 0, 0, false
	}
	parts := strings.Split(s, ".")
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

// parseUVDirVersion parses the version from a uv directory name like
// "cpython-3.14.2-macos-aarch64-none".
func parseUVDirVersion(name string) (major, minor int, ok bool) {
	rest := strings.TrimPrefix(name, "cpython-")
	if rest == name {
		return 0, 0, false
	}
	parts := strings.SplitN(rest, "-", 2)
	if len(parts) == 0 {
		return 0, 0, false
	}
	verParts := strings.SplitN(parts[0], ".", 3)
	if len(verParts) < 2 {
		return 0, 0, false
	}
	major, err1 := strconv.Atoi(verParts[0])
	minor, err2 := strconv.Atoi(verParts[1])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return major, minor, true
}

// libExtension returns the shared library file extension for the current OS.
func libExtension() string {
	if runtime.GOOS == "darwin" {
		return ".dylib"
	}
	return ".so"
}

// inferPythonHome attempts to derive PYTHONHOME from the library path.
// For a path like "/path/to/prefix/lib/libpython3.12.dylib",
// the home is "/path/to/prefix".
func inferPythonHome(libPath string) string {
	dir := filepath.Dir(libPath) // .../lib
	parent := filepath.Dir(dir)  // .../prefix
	// Verify this looks like a valid Python prefix by checking for
	// a lib/pythonX.Y directory.
	entries, err := filepath.Glob(filepath.Join(parent, "lib", "python3.*"))
	if err == nil && len(entries) > 0 {
		return parent
	}
	return ""
}

// detectUV scans ~/.local/share/uv/python/ for installed CPython versions.
func detectUV() []candidate {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	uvDir := filepath.Join(home, ".local", "share", "uv", "python")
	entries, err := os.ReadDir(uvDir)
	if err != nil {
		return nil
	}

	ext := libExtension()
	var results []candidate
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "cpython-") {
			continue
		}
		// Allow freethreaded variants.
		major, minor, ok := parseUVDirVersion(name)
		if !ok {
			continue
		}
		libName := fmt.Sprintf("libpython%d.%d%s", major, minor, ext)
		libPath := filepath.Join(uvDir, name, "lib", libName)
		if _, err := os.Stat(libPath); err == nil {
			results = append(results, candidate{path: libPath, major: major, minor: minor})
		}
	}
	return results
}
