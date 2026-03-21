//go:build windows

package pyffi

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// detectCandidates returns Python DLL paths on Windows.
// DLL naming: python314.dll (no dots in version).
func detectCandidates() []candidate {
	var results []candidate
	results = append(results, detectUV()...)
	results = append(results, detectWindowsRegistry()...)
	results = append(results, detectWindowsCommonPaths()...)
	return results
}

func defaultSearchPaths() []string {
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, ".local\\share\\uv\\python\\"),
		"C:\\Python3*\\python3*.dll",
		filepath.Join(home, "AppData\\Local\\Programs\\Python\\Python3*\\python3*.dll"),
	}
}

// detectWindowsRegistry searches common Windows Python install locations.
func detectWindowsRegistry() []candidate {
	var results []candidate
	// Common install locations.
	for minor := 14; minor >= 8; minor-- {
		dllName := fmt.Sprintf("python3%d.dll", minor)
		paths := []string{
			fmt.Sprintf("C:\\Python3%d\\%s", minor, dllName),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Python",
				fmt.Sprintf("Python3%d", minor), dllName),
		}
		for _, p := range paths {
			if fileExists(p) {
				results = append(results, candidate{path: p, major: 3, minor: minor})
			}
		}
	}
	return results
}

// detectWindowsCommonPaths searches PATH for python executables and derives DLL paths.
func detectWindowsCommonPaths() []candidate {
	var results []candidate
	pathDirs := filepath.SplitList(os.Getenv("PATH"))
	for _, dir := range pathDirs {
		for minor := 14; minor >= 8; minor-- {
			dllName := fmt.Sprintf("python3%d.dll", minor)
			p := filepath.Join(dir, dllName)
			if fileExists(p) {
				results = append(results, candidate{path: p, major: 3, minor: minor})
			}
		}
	}
	return results
}

// libExtension on Windows is overridden by the build tag in detect.go.
// Windows uses .dll but naming is different (python314.dll, no "lib" prefix, no dots).
func windowsDLLName(major, minor int) string {
	return "python" + strconv.Itoa(major) + strconv.Itoa(minor) + ".dll"
}
