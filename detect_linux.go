//go:build linux

package pyffi

import (
	"os"
	"path/filepath"
	"strings"
)

// detectCandidates returns Python shared library paths found on Linux.
func detectCandidates() []candidate {
	var results []candidate
	results = append(results, detectUV()...)
	results = append(results, detectSystemLinux()...)
	return results
}

func defaultSearchPaths() []string {
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, ".local/share/uv/python/"),
		"/usr/lib/libpython3.*.so*",
		"/usr/lib/x86_64-linux-gnu/libpython3.*.so*",
		"/usr/lib/aarch64-linux-gnu/libpython3.*.so*",
		"/usr/local/lib/libpython3.*.so*",
	}
}

func detectSystemLinux() []candidate {
	searchDirs := []string{
		"/usr/lib",
		"/usr/lib/x86_64-linux-gnu",
		"/usr/lib/aarch64-linux-gnu",
		"/usr/local/lib",
	}

	var results []candidate
	for _, dir := range searchDirs {
		matches, _ := filepath.Glob(filepath.Join(dir, "libpython3.*.so*"))
		for _, m := range matches {
			base := filepath.Base(m)
			// Skip deeply versioned symlinks like libpython3.12.so.1.0.
			if strings.Count(base, ".") > 3 {
				continue
			}
			if major, minor, ok := parseVersion(base); ok {
				results = append(results, candidate{path: m, major: major, minor: minor})
			}
		}
	}
	return results
}
