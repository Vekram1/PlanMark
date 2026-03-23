package build

import (
	"testing"

	"github.com/vikramoddiraju/planmark/internal/ir"
)

func TestTaskSemanticFingerprintDeterminism(t *testing.T) {
	base := ir.Task{
		ID:      "fixture.task",
		Title:   "Implement deterministic fingerprints",
		Horizon: "now",
		Deps:    []string{"dep.b", "dep.a"},
		Accept:  []string{"cmd:go test ./...", "text:output includes fingerprint"},
		Steps: []ir.TaskStep{
			{Title: "write code"},
			{Title: "run tests"},
		},
		EvidenceNodeRefs: []string{"node.b", "node.a"},
	}
	reordered := ir.Task{
		ID:      " fixture.task ",
		Title:   "Implement deterministic fingerprints",
		Horizon: "NOW",
		Deps:    []string{"dep.a", "dep.b"},
		Accept:  []string{"text:output includes fingerprint", "cmd:go test ./..."},
		Steps: []ir.TaskStep{
			{Title: "run tests"},
			{Title: "write code"},
		},
		EvidenceNodeRefs: []string{"node.a", "node.b"},
	}

	fpA := TaskSemanticFingerprint(base)
	fpB := TaskSemanticFingerprint(reordered)
	if fpA != fpB {
		t.Fatalf("expected deterministic fingerprint for equivalent semantics, got %q vs %q", fpA, fpB)
	}

	changed := reordered
	changed.Steps = append(changed.Steps, ir.TaskStep{Title: "review output"})
	fpC := TaskSemanticFingerprint(changed)
	if fpA == fpC {
		t.Fatalf("expected fingerprint change when semantic steps change")
	}
}
