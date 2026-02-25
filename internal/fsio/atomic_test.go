package fsio

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomicCreatesAndReplaces(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "sync", "manifest.json")

	if err := WriteFileAtomic(target, []byte("first\n"), 0o644); err != nil {
		t.Fatalf("first atomic write: %v", err)
	}
	first, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read first output: %v", err)
	}
	if string(first) != "first\n" {
		t.Fatalf("unexpected first file content: %q", string(first))
	}

	if err := WriteFileAtomic(target, []byte("second\n"), 0o644); err != nil {
		t.Fatalf("second atomic write: %v", err)
	}
	second, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read second output: %v", err)
	}
	if string(second) != "second\n" {
		t.Fatalf("unexpected second file content: %q", string(second))
	}
}
