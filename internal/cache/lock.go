package cache

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var ErrLockHeld = errors.New("lock already held")

var lockNamePattern = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

type FileLock struct {
	path string
	file *os.File
}

func AcquireLock(stateDir string, name string) (*FileLock, error) {
	resolvedStateDir := strings.TrimSpace(stateDir)
	if resolvedStateDir == "" {
		return nil, fmt.Errorf("state directory must not be empty")
	}
	resolvedName := sanitizeLockName(name)
	lockPath := filepath.Join(resolvedStateDir, "locks", resolvedName+".lock")

	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("%w: %s", ErrLockHeld, lockPath)
		}
		return nil, fmt.Errorf("create lock file: %w", err)
	}
	if _, err := fmt.Fprintf(f, "pid=%d\n", os.Getpid()); err != nil {
		_ = f.Close()
		_ = os.Remove(lockPath)
		return nil, fmt.Errorf("write lock metadata: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(lockPath)
		return nil, fmt.Errorf("sync lock metadata: %w", err)
	}

	return &FileLock{
		path: lockPath,
		file: f,
	}, nil
}

func (l *FileLock) Release() error {
	if l == nil {
		return nil
	}
	file := l.file
	path := l.path
	l.file = nil
	l.path = ""

	var errs []error
	if file != nil {
		if err := file.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if path != "" {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

func sanitizeLockName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "default"
	}
	safe := lockNamePattern.ReplaceAllString(trimmed, "-")
	safe = strings.Trim(safe, "-.")
	if safe == "" {
		return "default"
	}
	return safe
}
