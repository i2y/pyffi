//go:build !windows

package pyffi

import "github.com/ebitengine/purego"

func openLibrary(path string) (uintptr, error) {
	return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
}

func loadSymbol(handle uintptr, name string) (uintptr, error) {
	return purego.Dlsym(handle, name)
}
