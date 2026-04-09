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

func TestRunDispatchesContextOpenAndExplain(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := "# Overview\n- [ ] Task now\n  @id fixture.task.now\n  @horizon now\n  @accept cmd:go test ./...\n"
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"context", "--plan", planPath, "fixture.task.now", "--level", "L0"}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected context exit success, got %d stderr=%q", exit, errOut.String())
	}

	out.Reset()
	errOut.Reset()
	exit = Run([]string{"open", "--plan", planPath, "fixture.task.now"}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected open exit success, got %d stderr=%q", exit, errOut.String())
	}

	out.Reset()
	errOut.Reset()
	exit = Run([]string{"explain", "--plan", planPath, "fixture.task.now", "--format", "text"}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected explain exit success, got %d stderr=%q", exit, errOut.String())
	}
}

func TestContextJSONUsesProtocolEnvelope(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := "- [ ] Task now\n  @id fixture.task.now\n  @horizon now\n  @accept cmd:go test ./...\n"
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"context", "--plan", planPath, "--format", "json", "fixture.task.now", "--level", "L0"}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected context exit success, got %d stderr=%q", exit, errOut.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json output: %v output=%q", err, out.String())
	}
	if payload["schema_version"] != "v0.1" {
		t.Fatalf("expected schema_version v0.1, got %v", payload["schema_version"])
	}
	if payload["command"] != "context" {
		t.Fatalf("expected command context, got %v", payload["command"])
	}
	if payload["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", payload["status"])
	}
	if _, ok := payload["data"].(map[string]any); !ok {
		t.Fatalf("expected object data payload, got %T", payload["data"])
	}
}

func TestContextL2JSONIncludesClosure(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Root",
		"  @id fixture.task.root",
		"  @horizon later",
		"  @deps fixture.task.dep",
		"",
		"- [ ] Dep",
		"  @id fixture.task.dep",
		"  @horizon later",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"context", "--plan", planPath, "--format", "json", "fixture.task.root", "--level", "L2"}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected context exit success, got %d stderr=%q", exit, errOut.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json output: %v output=%q", err, out.String())
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %T", payload["data"])
	}
	closure, ok := data["closure"].([]any)
	if !ok {
		t.Fatalf("expected closure array in L2 output, got %T", data["closure"])
	}
	if len(closure) != 1 {
		t.Fatalf("expected 1 closure item, got %d", len(closure))
	}
}

func TestContextNeedJSONIncludesSelectionMetadata(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	targetDir := filepath.Join(tmp, "internal", "cli")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir touches target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "context.go"), []byte("package cli\n"), 0o644); err != nil {
		t.Fatalf("write touches target: %v", err)
	}
	planBody := strings.Join([]string{
		"## Root",
		"@id fixture.task.root",
		"@horizon later",
		"@deps fixture.task.dep",
		"@touches internal/cli/context.go:1-5",
		"",
		"Task-local rationale.",
		"",
		"## Dep",
		"@id fixture.task.dep",
		"@horizon later",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"context", "--plan", planPath, "--format", "json", "--need", "auto", "fixture.task.root"}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected context exit success, got %d stderr=%q", exit, errOut.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json output: %v output=%q", err, out.String())
	}
	data := payload["data"].(map[string]any)
	if data["need"] != "auto" {
		t.Fatalf("expected need auto, got %v", data["need"])
	}
	if data["query"] != "fixture.task.root" {
		t.Fatalf("expected query fixture.task.root, got %v", data["query"])
	}
	if data["selected_context_class"] != "task+files" {
		t.Fatalf("expected task+files selection, got %v", data["selected_context_class"])
	}
	if data["sufficient_for_need"] != true {
		t.Fatalf("expected sufficient_for_need=true, got %v", data["sufficient_for_need"])
	}
	if data["fallback_used"] != false {
		t.Fatalf("expected fallback_used=false, got %v", data["fallback_used"])
	}
	if data["full_plan_required"] != false {
		t.Fatalf("expected full_plan_required=false, got %v", data["full_plan_required"])
	}
	stats, ok := data["stats"].(map[string]any)
	if !ok {
		t.Fatalf("expected stats object in selected packet, got %T", data["stats"])
	}
	if stats["included_lines"] == nil || stats["estimated_token_count"] == nil {
		t.Fatalf("expected quantitative stats fields, got %#v", stats)
	}
	path, ok := stats["escalation_path"].([]any)
	if !ok || len(path) != 2 || path[0] != "task" || path[1] != "task+files" {
		t.Fatalf("expected task -> task+files escalation path, got %#v", stats["escalation_path"])
	}
	if _, ok := data["sections"].([]any); !ok {
		t.Fatalf("expected sections array in selected packet, got %T", data["sections"])
	}
	if _, ok := data["pins"].([]any); !ok {
		t.Fatalf("expected pins array in selected packet, got %T", data["pins"])
	}
	fileRefs, ok := data["included_file_refs"].([]any)
	if !ok || len(fileRefs) != 1 {
		t.Fatalf("expected included_file_refs array in selected packet, got %T %v", data["included_file_refs"], data["included_file_refs"])
	}
	firstFileRef, ok := fileRefs[0].(map[string]any)
	if !ok || firstFileRef["path"] == nil || firstFileRef["reason"] == nil {
		t.Fatalf("expected structured included_file_refs entry, got %#v", fileRefs[0])
	}
	if _, ok := data["closure"]; ok {
		t.Fatalf("expected auto selection to avoid dependency closure expansion, got %T", data["closure"])
	}
}

func TestContextNeedTextIncludesFallbackAndQueryFields(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	targetDir := filepath.Join(tmp, "internal", "cli")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir touches target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "context.go"), []byte("package cli\n"), 0o644); err != nil {
		t.Fatalf("write touches target: %v", err)
	}
	planBody := strings.Join([]string{
		"## Root",
		"@id fixture.task.root",
		"@horizon later",
		"@touches internal/cli/context.go:1-5",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"context", "--plan", planPath, "--need", "auto", "fixture.task.root"}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected context exit success, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(out.String(), "query: fixture.task.root") {
		t.Fatalf("expected text output to include query, got %q", out.String())
	}
	if !strings.Contains(out.String(), "fallback_used: false") {
		t.Fatalf("expected text output to include fallback_used, got %q", out.String())
	}
	if !strings.Contains(out.String(), "full_plan_required: false") {
		t.Fatalf("expected text output to include full_plan_required, got %q", out.String())
	}
	if !strings.Contains(out.String(), "stats.included_lines:") || !strings.Contains(out.String(), "stats.estimated_token_count:") {
		t.Fatalf("expected text output to include stats lines, got %q", out.String())
	}
	if !strings.Contains(out.String(), "included_file_refs: 1") {
		t.Fatalf("expected text output to include structured file-ref count, got %q", out.String())
	}
}

func TestContextDefaultsToNeedAutoWhenNoNeedOrLevelProvided(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	targetDir := filepath.Join(tmp, "internal", "cli")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir touches target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "context.go"), []byte("package cli\n"), 0o644); err != nil {
		t.Fatalf("write touches target: %v", err)
	}
	planBody := strings.Join([]string{
		"## Root",
		"@id fixture.task.root",
		"@horizon later",
		"@touches internal/cli/context.go:1-5",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"context", "--plan", planPath, "--format", "json", "fixture.task.root"}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected context exit success, got %d stderr=%q", exit, errOut.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json output: %v output=%q", err, out.String())
	}
	data := payload["data"].(map[string]any)
	if data["need"] != "auto" {
		t.Fatalf("expected implicit need auto, got %v", data["need"])
	}
	if data["selected_context_class"] != "task+files" {
		t.Fatalf("expected task+files selection, got %v", data["selected_context_class"])
	}
}

func TestContextRejectsNeedAndNonDefaultLevelTogether(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := "- [ ] Task now\n  @id fixture.task.now\n  @horizon now\n  @accept cmd:go test ./...\n"
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"context", "--plan", planPath, "--need", "execute", "--level", "L2", "fixture.task.now"}, &out, &errOut)
	if exit != protocol.ExitUsageError {
		t.Fatalf("expected usage error, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(errOut.String(), "--need and --level may not be combined") {
		t.Fatalf("expected explicit need/level conflict message, got %q", errOut.String())
	}
}

func TestContextRejectsNeedAndExplicitLevelL0Together(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := "- [ ] Task now\n  @id fixture.task.now\n  @horizon now\n  @accept cmd:go test ./...\n"
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"context", "--plan", planPath, "--need", "execute", "--level", "L0", "fixture.task.now"}, &out, &errOut)
	if exit != protocol.ExitUsageError {
		t.Fatalf("expected usage error, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(errOut.String(), "--need and --level may not be combined") {
		t.Fatalf("expected explicit need/level conflict message, got %q", errOut.String())
	}
}

func TestContextUsagePrefersNeedAndDemotesLevel(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"context"}, &out, &errOut)
	if exit != protocol.ExitUsageError {
		t.Fatalf("expected usage error, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(errOut.String(), "[--need execute|edit|dependency-check|handoff|auto]") {
		t.Fatalf("expected usage to advertise --need, got %q", errOut.String())
	}
	if !strings.Contains(errOut.String(), "legacy compatibility: --level L0|L1|L2") {
		t.Fatalf("expected usage to demote --level to compatibility path, got %q", errOut.String())
	}
}

func TestContextNeedTextOmitsLegacyLevelLine(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := "- [ ] Task now\n  @id fixture.task.now\n  @horizon now\n  @accept cmd:go test ./...\n"
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"context", "--plan", planPath, "--need", "execute", "fixture.task.now"}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected context exit success, got %d stderr=%q", exit, errOut.String())
	}
	if strings.Contains(out.String(), "\nlevel: ") {
		t.Fatalf("expected need-based text output to omit legacy level line, got %q", out.String())
	}
	if !strings.Contains(out.String(), "selected_context_class: task") {
		t.Fatalf("expected need-based text output to include selected_context_class, got %q", out.String())
	}
}

func TestRootHelpDescribesNeedBasedContextAndHandoff(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"--help"}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected root help exit success, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(out.String(), "context         Build a deterministic need-based task context packet") {
		t.Fatalf("expected root help to describe need-based context, got %q", out.String())
	}
	if !strings.Contains(out.String(), "handoff         Build a deterministic handoff packet for agent transfer") {
		t.Fatalf("expected root help to describe handoff as deterministic transfer packet, got %q", out.String())
	}
}
