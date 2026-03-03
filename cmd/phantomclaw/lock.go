package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func acquireSingleInstanceLock(lockPath string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err == nil {
		_, _ = lock.WriteString(strconv.Itoa(os.Getpid()) + "\n")
		_, _ = lock.WriteString(time.Now().UTC().Format(time.RFC3339) + "\n")
		return lock, nil
	}

	if os.IsExist(err) {
		return nil, fmt.Errorf("another PhantomClaw instance may already be running (lock exists: %s)", lockPath)
	}
	return nil, err
}

func releaseSingleInstanceLock(lock *os.File, lockPath string) {
	if lock != nil {
		_ = lock.Close()
	}
	if strings.TrimSpace(lockPath) != "" {
		_ = os.Remove(lockPath)
	}
}
