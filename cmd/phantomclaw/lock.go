package main

import (
	"bufio"
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
	for attempt := 0; attempt < 2; attempt++ {
		lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err == nil {
			_, _ = lock.WriteString(strconv.Itoa(os.Getpid()) + "\n")
			_, _ = lock.WriteString(time.Now().UTC().Format(time.RFC3339) + "\n")
			return lock, nil
		}

		if !os.IsExist(err) {
			return nil, err
		}

		pid, readErr := readLockPID(lockPath)
		if readErr != nil || pid <= 0 {
			return nil, fmt.Errorf("another PhantomClaw instance may already be running (lock exists: %s)", lockPath)
		}
		if processExists(pid) {
			return nil, fmt.Errorf("another PhantomClaw instance is running (pid=%d, lock=%s)", pid, lockPath)
		}

		if removeErr := os.Remove(lockPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return nil, fmt.Errorf("stale lock detected but cannot remove %s: %w", lockPath, removeErr)
		}
	}

	return nil, fmt.Errorf("failed to acquire lock at %s", lockPath)
}

func releaseSingleInstanceLock(lock *os.File, lockPath string) {
	if lock != nil {
		_ = lock.Close()
	}
	if strings.TrimSpace(lockPath) != "" {
		_ = os.Remove(lockPath)
	}
}

func readLockPID(lockPath string) (int, error) {
	f, err := os.Open(lockPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return 0, err
		}
		return 0, fmt.Errorf("empty lock file")
	}
	line := strings.TrimSpace(scanner.Text())
	pid, err := strconv.Atoi(line)
	if err != nil {
		return 0, err
	}
	return pid, nil
}
