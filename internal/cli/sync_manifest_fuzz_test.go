package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func FuzzLoadSyncManifestDeterminism(f *testing.F) {
	f.Add([]byte("{\"schema_version\":\"v0.1\",\"entries\":[{\"id\":\"task.a\"}]}\n"))
	f.Add([]byte("{\"entries\":[]}\n"))
	f.Add([]byte("not-json"))

	f.Fuzz(func(t *testing.T, payload []byte) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "sync", "beads-manifest.json")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir manifest dir: %v", err)
		}
		if err := os.WriteFile(path, payload, 0o644); err != nil {
			t.Fatalf("write manifest payload: %v", err)
		}

		first, errA := loadSyncManifest(path)
		second, errB := loadSyncManifest(path)
		if (errA != nil) != (errB != nil) {
			t.Fatalf("nondeterministic manifest error state: %v vs %v", errA, errB)
		}
		if errA != nil {
			if errA.Error() != errB.Error() {
				t.Fatalf("nondeterministic manifest error text: %q vs %q", errA.Error(), errB.Error())
			}
			return
		}
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("nondeterministic loaded manifest")
		}
	})
}
