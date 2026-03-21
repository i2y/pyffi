package pyffi_test

import (
	"strings"
	"testing"
)

func TestPythonVersion(t *testing.T) {
	rt := newOrSkip(t)

	t.Run("VersionString", func(t *testing.T) {
		v := rt.PythonVersion()
		if v == "" {
			t.Fatal("PythonVersion returned empty string")
		}
		if !strings.HasPrefix(v, "3.") {
			t.Errorf("PythonVersion = %q, expected to start with '3.'", v)
		}
	})

	t.Run("VersionInfo", func(t *testing.T) {
		major, minor, micro := rt.PythonVersionInfo()
		if major != 3 {
			t.Errorf("major = %d, want 3", major)
		}
		if minor < 8 {
			t.Errorf("minor = %d, expected >= 8", minor)
		}
		if micro < 0 {
			t.Errorf("micro = %d, expected >= 0", micro)
		}
	})
}
