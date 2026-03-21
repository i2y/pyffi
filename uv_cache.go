package pyffi

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// ClearCache removes all cached venvs created by Dependencies().
func ClearCache() error {
	return os.RemoveAll(cacheBaseDir())
}

// cacheBaseDir returns the base directory for cached venvs.
func cacheBaseDir() string {
	if d := os.Getenv("XDG_CACHE_HOME"); d != "" {
		return filepath.Join(d, "pyffi", "venvs")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "pyffi", "venvs")
}

// cacheVenvDir returns the directory for a specific cached venv.
func cacheVenvDir(hash string) string {
	return filepath.Join(cacheBaseDir(), hash)
}

// hashDeps computes a short hash from the dependency list and Python version.
func hashDeps(deps []string, major, minor int) string {
	sorted := slices.Clone(deps)
	slices.Sort(sorted)
	h := sha256.New()
	fmt.Fprintf(h, "python%d.%d\n", major, minor)
	for _, d := range sorted {
		fmt.Fprintf(h, "%s\n", d)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
