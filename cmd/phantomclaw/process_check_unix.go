//go:build !windows

package main

import (
	"errors"
	"os"
	"syscall"
)

func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	return !errors.Is(err, os.ErrProcessDone)
}
