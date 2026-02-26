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
	}
	reordered := ir.Task{
		ID:      " fixture.task ",
		Title:   "Implement deterministic fingerprints",
		Horizon: "NOW",
		Deps:    []string{"dep.a", "dep.b"},
		Accept:  []string{"text:output includes fingerprint", "cmd:go test ./..."},
	}

	fpA := TaskSemanticFingerprint(base)
	fpB := TaskSemanticFingerprint(reordered)
	if fpA != fpB {
		t.Fatalf("expected deterministic fingerprint for equivalent semantics, got %q vs %q", fpA, fpB)
	}

	changed := reordered
	changed.Accept = append(changed.Accept, "cmd:go test ./... -run TestExtra")
	fpC := TaskSemanticFingerprint(changed)
	if fpA == fpC {
		t.Fatalf("expected fingerprint change when semantic accept criteria changes")
	}
}
