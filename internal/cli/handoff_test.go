package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/protocol"
)

func TestHandoffPacketDeterministic(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"## Root task",
		"@id fixture.task.root",
		"@horizon now",
		"@accept cmd:go test ./...",
		"@deps fixture.task.dep",
		"",
		"- [ ] Root step",
		"### Evidence",
		"",
		"- [ ] Dep task",
		"  @id fixture.task.dep",
		"  @horizon next",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	runOnce := func() (string, string, int) {
		var out bytes.Buffer
		var errOut bytes.Buffer
		exit := Run([]string{"handoff", "fixture.task.root", "--plan", planPath, "--format", "json"}, &out, &errOut)
		return out.String(), errOut.String(), exit
	}

	out1, err1, exit1 := runOnce()
	out2, err2, exit2 := runOnce()
	if exit1 != protocol.ExitSuccess || exit2 != protocol.ExitSuccess {
		t.Fatalf("expected success exits, got %d/%d stderr1=%q stderr2=%q", exit1, exit2, err1, err2)
	}
	if out1 != out2 {
		t.Fatalf("expected deterministic JSON output\nfirst=%s\nsecond=%s", out1, out2)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(out1), &payload); err != nil {
		t.Fatalf("decode json output: %v output=%q", err, out1)
	}
	if payload["command"] != "handoff" {
		t.Fatalf("expected command=handoff, got %v", payload["command"])
	}
	data := payload["data"].(map[string]any)
	if data["plan_path"] != filepath.ToSlash(planPath) {
		t.Fatalf("expected plan_path %q, got %v", filepath.ToSlash(planPath), data["plan_path"])
	}
	if data["need"] != "handoff" {
		t.Fatalf("expected handoff need, got %v", data["need"])
	}
	if data["selected_context_class"] != "task" {
		t.Fatalf("expected bounded handoff class task, got %v", data["selected_context_class"])
	}
	if data["sufficient_for_need"] != true {
		t.Fatalf("expected sufficient_for_need=true, got %v", data["sufficient_for_need"])
	}
	if data["fallback_used"] != false || data["full_plan_required"] != false {
		t.Fatalf("expected fallback/full-plan flags false, got %v / %v", data["fallback_used"], data["full_plan_required"])
	}
	steps, ok := data["steps"].([]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("expected 1 step in handoff packet, got %v", data["steps"])
	}
	evidence, ok := data["evidence"].([]any)
	if !ok || len(evidence) != 1 {
		t.Fatalf("expected 1 evidence block in handoff packet, got %v", data["evidence"])
	}
	refs, ok := data["global_context_refs"].([]any)
	if !ok || len(refs) < 2 {
		t.Fatalf("expected global_context_refs, got %v", data["global_context_refs"])
	}
	if _, ok := data["next_upgrade"].(string); !ok {
		t.Fatalf("expected next_upgrade in handoff packet, got %T", data["next_upgrade"])
	}
	if _, ok := data["included_file_refs"]; ok {
		t.Fatalf("expected deps-only handoff packet to omit included_file_refs, got %T", data["included_file_refs"])
	}
	deps, ok := data["included_deps"].([]any)
	if !ok || len(deps) != 1 || deps[0] != "fixture.task.dep" {
		t.Fatalf("expected compatibility included_deps in handoff packet, got %v", data["included_deps"])
	}
	depRefs, ok := data["included_dep_refs"].([]any)
	if !ok || len(depRefs) != 1 {
		t.Fatalf("expected structured included_dep_refs in handoff packet, got %v", data["included_dep_refs"])
	}
	firstDepRef, ok := depRefs[0].(map[string]any)
	if !ok || firstDepRef["task_id"] != "fixture.task.dep" || firstDepRef["reason"] == nil {
		t.Fatalf("expected structured included_dep_refs entry, got %#v", depRefs[0])
	}
	stats, ok := data["stats"].(map[string]any)
	if !ok {
		t.Fatalf("expected stats object in handoff packet, got %T", data["stats"])
	}
	path, ok := stats["escalation_path"].([]any)
	if !ok || len(path) != 1 || path[0] != "task" {
		t.Fatalf("expected bounded handoff escalation path [task], got %#v", stats["escalation_path"])
	}
}

func TestHandoffUnknownTaskReturnsValidationFailed(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := "- [ ] Task\n  @id fixture.task\n"
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"handoff", "missing.task", "--plan", planPath}, &out, &errOut)
	if exit != protocol.ExitValidationFailed {
		t.Fatalf("expected validation failure, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(errOut.String(), "task not found") {
		t.Fatalf("expected task not found message, got %q", errOut.String())
	}
}

func TestHandoffTextIncludesStructuredRefCountsAndNextUpgrade(t *testing.T) {
	tmp := t.TempDir()
	docPath := filepath.Join(tmp, "docs", "specs", "context-selection-v0.1.md")
	if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
		t.Fatalf("mkdir doc path: %v", err)
	}
	if err := os.WriteFile(docPath, []byte("# spec\n"), 0o644); err != nil {
		t.Fatalf("write doc file: %v", err)
	}

	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"## Root task",
		"@id fixture.task.root",
		"@horizon now",
		"@accept cmd:test -f docs/specs/context-selection-v0.1.md",
		"@deps fixture.task.dep",
		"",
		"## Dep task",
		"@id fixture.task.dep",
		"@horizon next",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"handoff", "fixture.task.root", "--plan", planPath, "--format", "text"}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected handoff text success, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(out.String(), "included_file_refs: 1") {
		t.Fatalf("expected handoff text to include structured file-ref count, got %q", out.String())
	}
	if !strings.Contains(out.String(), "included_dep_refs: 0") {
		t.Fatalf("expected handoff text to include structured dep-ref count, got %q", out.String())
	}
	if !strings.Contains(out.String(), "next_upgrade: task+files+deps") {
		t.Fatalf("expected handoff text to include next_upgrade, got %q", out.String())
	}
}
