package doctor

import (
	"testing"

	"github.com/vikramoddiraju/planmark/internal/diag"
	"github.com/vikramoddiraju/planmark/internal/ir"
)

func TestBuildFixOutDeduplicatesTaskIDs(t *testing.T) {
	plan := ir.PlanIR{
		PlanPath: "PLAN.md",
		Semantic: ir.SemanticIR{
			Tasks: []ir.Task{
				{ID: "task.now", Horizon: "now"},
				{ID: "task.now", Horizon: "now"},
			},
		},
	}

	out := BuildFixOut(plan, "exec")
	if len(out.Fixes) != 1 {
		t.Fatalf("expected exactly one fix suggestion, got %d: %#v", len(out.Fixes), out.Fixes)
	}
	if out.Fixes[0].TaskID != "task.now" {
		t.Fatalf("expected task id task.now, got %q", out.Fixes[0].TaskID)
	}
	if out.Fixes[0].Code != diag.CodeMissingAccept {
		t.Fatalf("expected code %q, got %q", diag.CodeMissingAccept, out.Fixes[0].Code)
	}
}
