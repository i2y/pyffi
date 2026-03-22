//go:build windows

package pyffi

import (
	"fmt"
	"syscall"
	"unsafe"
)

func openLibrary(path string) (uintptr, error) {
	h, err := syscall.LoadLibrary(path)
	if err != nil {
		return 0, fmt.Errorf("LoadLibrary(%s): %w", path, err)
	}
	return uintptr(h), nil
}

func loadSymbol(handle uintptr, name string) (uintptr, error) {
	namePtr, err := syscall.BytePtrFromString(name)
	if err != nil {
		return 0, err
	}
	addr, _, err := syscall.SyscallN(
		procGetProcAddress.Addr(),
		handle,
		uintptr(unsafe.Pointer(namePtr)),
	)
	if addr == 0 {
		return 0, fmt.Errorf("GetProcAddress(%s): %w", name, err)
	}
	return addr, nil
}

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procGetProcAddress = kernel32.NewProc("GetProcAddress")
)
