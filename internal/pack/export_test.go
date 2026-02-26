package pack_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/pack"
)

func TestPackIndexDeterministic(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := "- [ ] Task A\n  @id fixture.task.a\n  @horizon now\n  @accept cmd:echo ok\n"
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	outA := filepath.Join(tmp, "out-a")
	outB := filepath.Join(tmp, "out-b")
	if _, err := pack.Export(pack.Options{
		PlanPath: planPath,
		OutPath:  outA,
		Levels:   []string{"L0"},
	}); err != nil {
		t.Fatalf("first export: %v", err)
	}
	if _, err := pack.Export(pack.Options{
		PlanPath: planPath,
		OutPath:  outB,
		Levels:   []string{"L0"},
	}); err != nil {
		t.Fatalf("second export: %v", err)
	}

	indexA, err := os.ReadFile(filepath.Join(outA, "pack", "index.json"))
	if err != nil {
		t.Fatalf("read index A: %v", err)
	}
	indexB, err := os.ReadFile(filepath.Join(outB, "pack", "index.json"))
	if err != nil {
		t.Fatalf("read index B: %v", err)
	}
	if !bytes.Equal(indexA, indexB) {
		t.Fatalf("expected deterministic index bytes across exports")
	}
}

func TestPackContainsPlanAndManifest(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := "- [ ] Task A\n  @id fixture.task.a\n  @horizon now\n  @accept cmd:echo ok\n"
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	outDir := filepath.Join(tmp, "out")
	result, err := pack.Export(pack.Options{
		PlanPath: planPath,
		OutPath:  outDir,
		Levels:   []string{"L0"},
	})
	if err != nil {
		t.Fatalf("export pack: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "pack", "plan", "plan.json")); err != nil {
		t.Fatalf("expected plan json in pack: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "pack", "plan", "compile-manifest.json")); err != nil {
		t.Fatalf("expected compile manifest in pack: %v", err)
	}
	if result.TaskCount != 1 {
		t.Fatalf("expected one task in pack, got %d", result.TaskCount)
	}

	indexBytes, err := os.ReadFile(filepath.Join(outDir, "pack", "index.json"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	var index map[string]any
	if err := json.Unmarshal(indexBytes, &index); err != nil {
		t.Fatalf("unmarshal index: %v", err)
	}
	blobs, ok := index["blobs"].([]any)
	if !ok || len(blobs) < 2 {
		t.Fatalf("expected blobs in index, got: %v", index["blobs"])
	}
}

func TestPackTarGzContainsIndex(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := "- [ ] Task A\n  @id fixture.task.a\n  @horizon now\n  @accept cmd:echo ok\n"
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	outTar := filepath.Join(tmp, "bundle.tar.gz")
	result, err := pack.Export(pack.Options{
		PlanPath: planPath,
		OutPath:  outTar,
		Levels:   []string{"L0"},
	})
	if err != nil {
		t.Fatalf("export pack tar.gz: %v", err)
	}
	if result.IndexPath != "pack/index.json" {
		t.Fatalf("expected tar index path pack/index.json, got %q", result.IndexPath)
	}

	f, err := os.Open(outTar)
	if err != nil {
		t.Fatalf("open tar.gz: %v", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gr.Close()
	tr := tar.NewReader(gr)

	foundIndex := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read tar entry: %v", err)
		}
		if hdr.Name == "pack/index.json" {
			foundIndex = true
			break
		}
	}
	if !foundIndex {
		t.Fatalf("expected pack/index.json in tarball")
	}
}
