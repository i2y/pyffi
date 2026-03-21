//go:build darwin

package pyffi

import (
	"os"
	"path/filepath"
	"runtime"
)

// detectCandidates returns Python shared library paths found on macOS.
// Search order (highest priority first):
//  1. uv-managed installations (~/.local/share/uv/python/)
//  2. Homebrew (arm64: /opt/homebrew, amd64: /usr/local)
//  3. python.org Framework (/Library/Frameworks/Python.framework)
//  4. System/Xcode Python (last resort)
func detectCandidates() []candidate {
	var results []candidate
	results = append(results, detectUV()...)
	results = append(results, detectHomebrew()...)
	results = append(results, detectFramework()...)
	results = append(results, detectSystemDarwin()...)
	return results
}

func defaultSearchPaths() []string {
	home, _ := os.UserHomeDir()
	prefix := homebrewPrefix()
	return []string{
		filepath.Join(home, ".local/share/uv/python/"),
		filepath.Join(prefix, "Frameworks/Python.framework/Versions/*/lib/libpython3.*.dylib"),
		filepath.Join(prefix, "lib/libpython3.*.dylib"),
		"/Library/Frameworks/Python.framework/Versions/*/lib/libpython3.*.dylib",
	}
}

func homebrewPrefix() string {
	if runtime.GOARCH == "arm64" {
		return "/opt/homebrew"
	}
	return "/usr/local"
}

func detectHomebrew() []candidate {
	prefix := homebrewPrefix()
	var results []candidate

	// Homebrew Framework layout.
	frameworkBase := filepath.Join(prefix, "Frameworks", "Python.framework", "Versions")
	matches, _ := filepath.Glob(filepath.Join(frameworkBase, "3.*", "lib", "libpython3.*.dylib"))
	for _, m := range matches {
		if major, minor, ok := parseVersion(filepath.Base(m)); ok {
			results = append(results, candidate{path: m, major: major, minor: minor})
		}
	}

	// Homebrew lib layout.
	matches, _ = filepath.Glob(filepath.Join(prefix, "lib", "libpython3.*.dylib"))
	for _, m := range matches {
		if major, minor, ok := parseVersion(filepath.Base(m)); ok {
			results = append(results, candidate{path: m, major: major, minor: minor})
		}
	}

	return results
}

func detectFramework() []candidate {
	var results []candidate
	matches, _ := filepath.Glob("/Library/Frameworks/Python.framework/Versions/3.*/lib/libpython3.*.dylib")
	for _, m := range matches {
		if major, minor, ok := parseVersion(filepath.Base(m)); ok {
			results = append(results, candidate{path: m, major: major, minor: minor})
		}
	}
	return results
}

func detectSystemDarwin() []candidate {
	var results []candidate
	// Xcode Command Line Tools Python.
	matches, _ := filepath.Glob("/Library/Developer/CommandLineTools/Library/Frameworks/Python3.framework/Versions/3.*/lib/libpython3.*.dylib")
	for _, m := range matches {
		if major, minor, ok := parseVersion(filepath.Base(m)); ok {
			results = append(results, candidate{path: m, major: major, minor: minor})
		}
	}
	return results
}
