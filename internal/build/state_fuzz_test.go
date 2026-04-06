package build

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func FuzzCompileManifestPathDeterminism(f *testing.F) {
	f.Add(".planmark")
	f.Add("  ")

	f.Fuzz(func(t *testing.T, stateDir string) {
		pathA, errA := CompileManifestPath(stateDir)
		pathB, errB := CompileManifestPath(stateDir)
		if (errA != nil) != (errB != nil) {
			t.Fatalf("nondeterministic manifest path error state: %v vs %v", errA, errB)
		}
		if errA != nil {
			if errA.Error() != errB.Error() {
				t.Fatalf("nondeterministic manifest path error text: %q vs %q", errA.Error(), errB.Error())
			}
			return
		}
		if pathA != pathB {
			t.Fatalf("nondeterministic manifest path: %q vs %q", pathA, pathB)
		}
	})
}

func FuzzReadCompileManifestDeterminism(f *testing.F) {
	f.Add([]byte("{\"schema_version\":\"v0.1\",\"compile_id\":\"abc\"}\n"))
	f.Add([]byte("not-json"))

	f.Fuzz(func(t *testing.T, payload []byte) {
		tmp := t.TempDir()
		manifestPath := filepath.Join(tmp, "build", "compile-manifest.json")
		if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
			t.Fatalf("mkdir build dir: %v", err)
		}
		if err := os.WriteFile(manifestPath, payload, 0o644); err != nil {
			t.Fatalf("write manifest payload: %v", err)
		}

		first, errA := ReadCompileManifest(tmp)
		second, errB := ReadCompileManifest(tmp)
		if (errA != nil) != (errB != nil) {
			t.Fatalf("nondeterministic manifest read error state: %v vs %v", errA, errB)
		}
		if errA != nil {
			if errA.Error() != errB.Error() {
				t.Fatalf("nondeterministic manifest read error text: %q vs %q", errA.Error(), errB.Error())
			}
			return
		}
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("nondeterministic manifest read result")
		}
	})
}
