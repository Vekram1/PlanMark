package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func resolveRepoScopedPath(repoRoot string, targetPath string) (normalizedPath string, absPath string, err error) {
	rootAbs, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", "", fmt.Errorf("resolve repo root: %w", err)
	}
	rootResolved := rootAbs
	if resolved, err := filepath.EvalSymlinks(rootAbs); err == nil {
		rootResolved = resolved
	}

	cleanPath := filepath.Clean(strings.TrimSpace(filepath.FromSlash(targetPath)))
	if cleanPath == "" || cleanPath == "." {
		return "", "", fmt.Errorf("target path is empty")
	}
	if filepath.IsAbs(cleanPath) {
		return "", "", fmt.Errorf("target path must be relative")
	}

	absPath = filepath.Join(rootAbs, cleanPath)
	if !isWithinRoot(rootAbs, absPath) {
		return "", "", fmt.Errorf("target path escapes repository root")
	}

	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		if !isWithinRoot(rootResolved, resolved) {
			return "", "", fmt.Errorf("target path resolves outside repository root via symlink")
		}
	} else if !os.IsNotExist(err) {
		return "", "", fmt.Errorf("resolve symlinks: %w", err)
	}

	return filepath.ToSlash(cleanPath), absPath, nil
}

func isWithinRoot(rootAbs string, candidateAbs string) bool {
	rel, err := filepath.Rel(rootAbs, candidateAbs)
	if err != nil {
		return false
	}
	rel = filepath.Clean(rel)
	if rel == ".." {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
