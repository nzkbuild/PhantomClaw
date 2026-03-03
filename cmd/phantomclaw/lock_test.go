package main

import (
	"path/filepath"
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
