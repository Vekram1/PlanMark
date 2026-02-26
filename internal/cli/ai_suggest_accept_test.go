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

func TestAISuggestAcceptTextOutput(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task now",
		"  @id fixture.task.now",
		"  @horizon now",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"ai", "suggest-accept", "fixture.task.now", "--plan", planPath}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected success, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(out.String(), "suggestions:") {
		t.Fatalf("expected suggestions section, got %q", out.String())
	}
	if !strings.Contains(out.String(), "@accept cmd:<command>") {
		t.Fatalf("expected command suggestion, got %q", out.String())
	}
}

func TestAISuggestAcceptJSONOutput(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task now",
		"  @id fixture.task.now",
		"  @horizon now",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"ai", "suggest-accept", "fixture.task.now", "--plan", planPath, "--format", "json"}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected success, got %d stderr=%q", exit, errOut.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode output json: %v output=%q", err, out.String())
	}
	if payload["command"] != "ai suggest-accept" {
		t.Fatalf("expected command field, got %v", payload["command"])
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object data payload, got %T", payload["data"])
	}
	suggestions, ok := data["suggestions"].([]any)
	if !ok || len(suggestions) == 0 {
		t.Fatalf("expected non-empty suggestions, got %v", data["suggestions"])
	}
}

func TestAISuggestAcceptMissingTaskReturnsValidationFailure(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task next",
		"  @id fixture.task.next",
		"  @horizon next",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"ai", "suggest-accept", "missing.task", "--plan", planPath}, &out, &errOut)
	if exit != protocol.ExitValidationFailed {
		t.Fatalf("expected validation failure, got %d stderr=%q output=%q", exit, errOut.String(), out.String())
	}
	if !strings.Contains(errOut.String(), "task not found") {
		t.Fatalf("expected task-not-found error, got %q", errOut.String())
	}
}

func TestAIUnknownSubcommandReturnsUsageError(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"ai", "draft-beads"}, &out, &errOut)
	if exit != protocol.ExitUsageError {
		t.Fatalf("expected usage error, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(errOut.String(), "unknown ai command") {
		t.Fatalf("expected unknown command message, got %q", errOut.String())
	}
}
