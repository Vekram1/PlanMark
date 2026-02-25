package doctor

import (
	"strings"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/diag"
	"github.com/vikramoddiraju/planmark/internal/ir"
)

func TestDiagnosticFormat(t *testing.T) {
	diagnostics := []diag.Diagnostic{
		{
			Severity: diag.SeverityWarning,
			Code:     diag.CodeUnattachedMetadata,
			Message:  "metadata could not be attached deterministically",
			Source: diag.SourceSpan{
				Path:      "PLAN.md",
				StartLine: 12,
				EndLine:   12,
			},
		},
		{
			Severity: diag.SeverityError,
			Code:     diag.CodePathTraversalReject,
			Message:  "pin path escapes repository root",
			Source: diag.SourceSpan{
				Path:      "PLAN.md",
				StartLine: 40,
				EndLine:   40,
			},
		},
	}

	rendered := FormatDiagnosticsText(diagnostics)
	if !strings.Contains(rendered, "UNATTACHED_METADATA") {
		t.Fatalf("expected stable code UNATTACHED_METADATA in rendered output, got: %q", rendered)
	}
	if !strings.Contains(rendered, "PATH_TRAVERSAL_REJECTED") {
		t.Fatalf("expected stable code PATH_TRAVERSAL_REJECTED in rendered output, got: %q", rendered)
	}
	if !strings.Contains(rendered, "PLAN.md:12-12") || !strings.Contains(rendered, "PLAN.md:40-40") {
		t.Fatalf("expected source path and line ranges in rendered output, got: %q", rendered)
	}

	errorPos := strings.Index(rendered, "PATH_TRAVERSAL_REJECTED")
	warnPos := strings.Index(rendered, "UNATTACHED_METADATA")
	if errorPos < 0 || warnPos < 0 {
		t.Fatalf("expected both diagnostic codes in output, got: %q", rendered)
	}
	if errorPos > warnPos {
		t.Fatalf("expected error diagnostics sorted before warnings; output=%q", rendered)
	}
}

func TestIDUniqueness(t *testing.T) {
	plan := ir.PlanIR{
		PlanPath: "PLAN.md",
		Semantic: ir.SemanticIR{
			Tasks: []ir.Task{
				{ID: "task.a", NodeRef: "n1", Title: "Task A"},
				{ID: "task.a", NodeRef: "n2", Title: "Task A duplicate"},
				{ID: "task.b", NodeRef: "n3", Title: "Task B", Deps: []string{"task.missing"}},
			},
		},
	}

	result := ValidateGraph(plan)
	if len(result.Diagnostics) == 0 {
		t.Fatalf("expected diagnostics for duplicate id and unknown dep")
	}

	if !containsCode(result.Diagnostics, diag.CodeDuplicateTaskID) {
		t.Fatalf("expected diagnostic code %s; got=%#v", diag.CodeDuplicateTaskID, result.Diagnostics)
	}
	if !containsCode(result.Diagnostics, diag.CodeUnknownDependency) {
		t.Fatalf("expected diagnostic code %s; got=%#v", diag.CodeUnknownDependency, result.Diagnostics)
	}
}

func TestDepCycleDetection(t *testing.T) {
	plan := ir.PlanIR{
		PlanPath: "PLAN.md",
		Semantic: ir.SemanticIR{
			Tasks: []ir.Task{
				{ID: "task.a", NodeRef: "n1", Title: "Task A", Deps: []string{"task.b"}},
				{ID: "task.b", NodeRef: "n2", Title: "Task B", Deps: []string{"task.a"}},
			},
		},
	}

	result := ValidateGraph(plan)
	if !containsCode(result.Diagnostics, diag.CodeDependencyCycle) {
		t.Fatalf("expected diagnostic code %s; got=%#v", diag.CodeDependencyCycle, result.Diagnostics)
	}
}

func TestHorizonReadinessRules(t *testing.T) {
	plan := ir.PlanIR{
		PlanPath: "PLAN.md",
		Semantic: ir.SemanticIR{
			Tasks: []ir.Task{
				{ID: "task.now", Horizon: "now", Accept: nil},
				{ID: "task.next", Horizon: "next", Accept: nil},
				{ID: "task.later", Horizon: "later", Accept: nil},
				{ID: "task.unknown", Horizon: "someday", Accept: nil},
			},
		},
	}

	diagnostics := EvaluateHorizonReadiness(plan)
	if !containsCode(diagnostics, diag.CodeMissingAccept) {
		t.Fatalf("expected %s diagnostics; got %#v", diag.CodeMissingAccept, diagnostics)
	}
	if !containsCode(diagnostics, diag.CodeUnknownHorizon) {
		t.Fatalf("expected %s diagnostics; got %#v", diag.CodeUnknownHorizon, diagnostics)
	}

	if !containsMessage(diagnostics, "horizon=now task \"task.now\" requires at least one @accept") {
		t.Fatalf("expected hard error for horizon=now missing accept; got %#v", diagnostics)
	}
	if !containsMessage(diagnostics, "horizon=next task \"task.next\" should define @accept (recommended)") {
		t.Fatalf("expected warning for horizon=next missing accept; got %#v", diagnostics)
	}
	if containsMessage(diagnostics, "task.later") {
		t.Fatalf("did not expect horizon=later missing-accept diagnostic; got %#v", diagnostics)
	}
}

func TestHorizonReadinessLooseProfileDowngradesNowMissingAccept(t *testing.T) {
	plan := ir.PlanIR{
		PlanPath: "PLAN.md",
		Semantic: ir.SemanticIR{
			Tasks: []ir.Task{
				{ID: "task.now", Horizon: "now", Accept: nil},
			},
		},
	}

	diagnostics := EvaluateHorizonReadinessWithProfile(plan, "loose")
	if len(diagnostics) != 1 {
		t.Fatalf("expected one diagnostic, got %#v", diagnostics)
	}
	if diagnostics[0].Code != diag.CodeMissingAccept {
		t.Fatalf("expected MISSING_ACCEPT, got %#v", diagnostics[0])
	}
	if diagnostics[0].Severity != diag.SeverityWarning {
		t.Fatalf("expected warning severity in loose profile, got %q", diagnostics[0].Severity)
	}
}

func TestRunRejectsInvalidProfile(t *testing.T) {
	_, err := Run(ir.PlanIR{}, "invalid")
	if err == nil {
		t.Fatalf("expected invalid profile error")
	}
}

func containsCode(diagnostics []diag.Diagnostic, code diag.Code) bool {
	for _, d := range diagnostics {
		if d.Code == code {
			return true
		}
	}
	return false
}

func containsMessage(diagnostics []diag.Diagnostic, substr string) bool {
	for _, d := range diagnostics {
		if strings.Contains(d.Message, substr) {
			return true
		}
	}
	return false
}
