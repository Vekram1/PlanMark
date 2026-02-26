package ir_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/compile"
)

func TestGoldenIRStability(t *testing.T) {
	planPath := filepath.ToSlash(filepath.Join("testdata", "plans", "mixed.md"))
	goldenPath := repoPath("testdata", "golden", "mixed.ir.golden.json")

	got := compileFixtureJSON(t, planPath)
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenPath, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("golden mismatch for %s", planPath)
	}
}

func TestCompileDeterminismOnFixtures(t *testing.T) {
	entries, err := os.ReadDir(repoPath("testdata", "plans"))
	if err != nil {
		t.Fatalf("read fixture dir: %v", err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		paths = append(paths, filepath.ToSlash(filepath.Join("testdata", "plans", entry.Name())))
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		t.Fatalf("expected at least one plan fixture")
	}

	for _, planPath := range paths {
		planPath := planPath
		t.Run(filepath.Base(planPath), func(t *testing.T) {
			first := compileFixtureJSON(t, planPath)
			second := compileFixtureJSON(t, planPath)
			if !bytes.Equal(first, second) {
				t.Fatalf("non-deterministic compile output for %s", planPath)
			}
		})
	}
}

func repoPath(parts ...string) string {
	return filepath.Join(append([]string{"..", ".."}, parts...)...)
}

func compileFixtureJSON(t *testing.T, planPath string) []byte {
	t.Helper()

	content, err := os.ReadFile(repoPath(filepath.FromSlash(planPath)))
	if err != nil {
		t.Fatalf("read fixture %s: %v", planPath, err)
	}
	irDoc, err := compile.CompilePlan(planPath, content, compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile fixture %s: %v", planPath, err)
	}
	payload, err := json.MarshalIndent(irDoc, "", "  ")
	if err != nil {
		t.Fatalf("encode fixture %s: %v", planPath, err)
	}
	return append(payload, '\n')
}
