package cache

import (
	"errors"
	"testing"
)

func TestCacheLocking(t *testing.T) {
	stateDir := t.TempDir()

	first, err := AcquireLock(stateDir, "context-packet")
	if err != nil {
		t.Fatalf("acquire first lock: %v", err)
	}

	_, err = AcquireLock(stateDir, "context-packet")
	if err == nil {
		t.Fatalf("expected second lock acquire to fail while first is held")
	}
	if !errors.Is(err, ErrLockHeld) {
		t.Fatalf("expected ErrLockHeld, got %v", err)
	}

	if err := first.Release(); err != nil {
		t.Fatalf("release first lock: %v", err)
	}

	second, err := AcquireLock(stateDir, "context-packet")
	if err != nil {
		t.Fatalf("acquire second lock after release: %v", err)
	}
	if err := second.Release(); err != nil {
		t.Fatalf("release second lock: %v", err)
	}
}

func TestCacheLockReleaseIdempotent(t *testing.T) {
	stateDir := t.TempDir()
	lock, err := AcquireLock(stateDir, "idempotent")
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("first release: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("second release should be no-op, got: %v", err)
	}
}
