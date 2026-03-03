package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestSingleInstanceLockLifecycle(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "phantomclaw.lock")

	lock, err := acquireSingleInstanceLock(lockPath)
	if err != nil {
		t.Fatalf("acquireSingleInstanceLock first: %v", err)
	}

	if _, err := acquireSingleInstanceLock(lockPath); err == nil {
		t.Fatal("expected second lock acquisition to fail while first lock is held")
	}

	releaseSingleInstanceLock(lock, lockPath)

	lock2, err := acquireSingleInstanceLock(lockPath)
	if err != nil {
		t.Fatalf("acquireSingleInstanceLock after release: %v", err)
	}
	releaseSingleInstanceLock(lock2, lockPath)
}

func TestSingleInstanceLockRecoversFromStalePID(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "phantomclaw.lock")
	// Very large PID to avoid collision with a real process.
	if err := os.WriteFile(lockPath, []byte("99999999\n2026-03-04T00:00:00Z\n"), 0644); err != nil {
		t.Fatalf("seed stale lock: %v", err)
	}

	lock, err := acquireSingleInstanceLock(lockPath)
	if err != nil {
		t.Fatalf("acquireSingleInstanceLock stale recovery: %v", err)
	}
	releaseSingleInstanceLock(lock, lockPath)
}

func TestSingleInstanceLockRejectsRunningPID(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "phantomclaw.lock")
	payload := []byte(strconv.Itoa(os.Getpid()) + "\n2026-03-04T00:00:00Z\n")
	if err := os.WriteFile(lockPath, payload, 0644); err != nil {
		t.Fatalf("seed active lock: %v", err)
	}

	if _, err := acquireSingleInstanceLock(lockPath); err == nil {
		t.Fatal("expected acquisition to fail when lock points to active PID")
	}
}
