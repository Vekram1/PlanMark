package build_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/build"
	"github.com/vikramoddiraju/planmark/internal/compile"
)

func TestBuildManifestDeterminism(t *testing.T) {
	planPath := filepath.Join("..", "..", "testdata", "plans", "mixed.md")
	content, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("read plan: %v", err)
	}

	compiled, err := compile.CompilePlan(filepath.ToSlash(planPath), content, compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}
	planJSON, err := json.MarshalIndent(compiled, "", "  ")
	if err != nil {
		t.Fatalf("marshal compiled plan: %v", err)
	}
	planJSON = append(planJSON, '\n')

	manifestA := build.BuildCompileManifest(compiled, content, planJSON, build.DefaultEffectiveConfigHash())
	manifestB := build.BuildCompileManifest(compiled, content, planJSON, build.DefaultEffectiveConfigHash())

	a, err := json.MarshalIndent(manifestA, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest A: %v", err)
	}
	b, err := json.MarshalIndent(manifestB, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest B: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("expected deterministic manifest bytes")
	}

	if manifestA.CompileID == "" {
		t.Fatalf("expected compile_id")
	}
	if manifestA.EffectiveConfigHash == "" {
		t.Fatalf("expected effective_config_hash")
	}
	if manifestA.SourceHashSummary.AggregateHash == "" {
		t.Fatalf("expected source hash summary aggregate hash")
	}
	if len(manifestA.TaskSemanticFingerprints) == 0 {
		t.Fatalf("expected per-task semantic fingerprints")
	}
}
