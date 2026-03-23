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
