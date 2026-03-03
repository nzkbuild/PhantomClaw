//go:build windows

package main

import (
	"errors"

	"golang.org/x/sys/windows"
)

func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err == nil {
		_ = windows.CloseHandle(handle)
		return true
	}
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return false
	}
	// Access denied and similar errors usually mean the process exists but is protected.
	return true
}
