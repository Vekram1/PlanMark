package fsio

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func FuzzWriteFileAtomicRoundTrip(f *testing.F) {
	f.Add("nested/out.txt", []byte("hello\n"), uint32(0o644))
	f.Add("deep/path/file.bin", []byte(""), uint32(0o600))

	f.Fuzz(func(t *testing.T, relPath string, payload []byte, permRaw uint32) {
		tmp := t.TempDir()
		cleanRel := filepath.Clean(relPath)
		if cleanRel == "." || cleanRel == ".." || filepath.IsAbs(cleanRel) {
			return
		}
		if len(cleanRel) >= 3 && cleanRel[:3] == ".."+string(os.PathSeparator) {
			return
		}

		target := filepath.Join(tmp, cleanRel)
		perm := os.FileMode(permRaw & 0o777)
		if perm == 0 {
			perm = 0o644
		}

		if err := WriteFileAtomic(target, payload, perm); err != nil {
			t.Fatalf("write file atomic: %v", err)
		}
		got, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("read file atomic output: %v", err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("payload mismatch after atomic write")
		}

		reversed := append([]byte(nil), payload...)
		for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
			reversed[i], reversed[j] = reversed[j], reversed[i]
		}
		if err := WriteFileAtomic(target, reversed, perm); err != nil {
			t.Fatalf("rewrite file atomic: %v", err)
		}
		got, err = os.ReadFile(target)
		if err != nil {
			t.Fatalf("read rewritten file: %v", err)
		}
		if !bytes.Equal(got, reversed) {
			t.Fatalf("rewrite payload mismatch after atomic write")
		}
	})
}
