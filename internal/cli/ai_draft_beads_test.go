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

func TestAIDraftBeadsTextOutput(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Now task",
		"  @id fixture.task.now",
		"  @horizon now",
		"  @accept cmd:echo ok",
		"",
		"- [ ] Next task",
		"  @id fixture.task.next",
		"  @horizon next",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"ai", "draft-beads", "--plan", planPath, "--limit", "1"}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected success, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(out.String(), "suggestion_count: 1") {
		t.Fatalf("expected suggestion_count=1, got %q", out.String())
	}
	if !strings.Contains(out.String(), "[fixture.task.next]") && !strings.Contains(out.String(), "[fixture.task.now]") {
		t.Fatalf("expected drafted entry with task id, got %q", out.String())
	}
	if strings.Contains(out.String(), "[fixture.task.now] [fixture.task.now]") ||
		strings.Contains(out.String(), "[fixture.task.next] [fixture.task.next]") {
		t.Fatalf("expected suggested title to be printed once, got %q", out.String())
	}
}

func TestAIDraftBeadsJSONOutputWithHorizonFilter(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Later task",
		"  @id fixture.task.later",
		"  @horizon later",
		"",
		"- [ ] Now task",
		"  @id fixture.task.now",
		"  @horizon now",
		"  @accept cmd:echo ok",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"ai", "draft-beads", "--plan", planPath, "--horizon", "now", "--format", "json"}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected success, got %d stderr=%q", exit, errOut.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode output json: %v output=%q", err, out.String())
	}
	if payload["command"] != "ai draft-beads" {
		t.Fatalf("expected command field, got %v", payload["command"])
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object data payload, got %T", payload["data"])
	}
	if data["horizon_filter"] != "now" {
		t.Fatalf("expected horizon filter now, got %v", data["horizon_filter"])
	}
	if count, _ := data["total_scanned"].(float64); int(count) != 1 {
		t.Fatalf("expected 1 scanned task for horizon filter, got %v", data["total_scanned"])
	}
	suggestions, ok := data["suggestions"].([]any)
	if !ok || len(suggestions) != 1 {
		t.Fatalf("expected exactly one suggestion, got %v", data["suggestions"])
	}
	first := suggestions[0].(map[string]any)
	if first["task_id"] != "fixture.task.now" {
		t.Fatalf("expected now task suggestion, got %v", first["task_id"])
	}
}

func TestAIDraftBeadsInvalidHorizonReturnsUsageError(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"ai", "draft-beads", "--plan", "PLAN.md", "--horizon", "soon"}, &out, &errOut)
	if exit != protocol.ExitUsageError {
		t.Fatalf("expected usage error, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(errOut.String(), "invalid --horizon value") {
		t.Fatalf("expected invalid horizon message, got %q", errOut.String())
	}
}

func TestAIDraftBeadsPrioritizesNowBeforeNextBeforeLater(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Later task",
		"  @id fixture.task.later",
		"  @horizon later",
		"",
		"- [ ] Now task",
		"  @id fixture.task.now",
		"  @horizon now",
		"",
		"- [ ] Next task",
		"  @id fixture.task.next",
		"  @horizon next",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"ai", "draft-beads", "--plan", planPath, "--limit", "1", "--format", "json"}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected success, got %d stderr=%q", exit, errOut.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode output json: %v output=%q", err, out.String())
	}
	data := payload["data"].(map[string]any)
	suggestions := data["suggestions"].([]any)
	first := suggestions[0].(map[string]any)
	if first["task_id"] != "fixture.task.now" {
		t.Fatalf("expected now task to be prioritized, got %v", first["task_id"])
	}
}

func TestAIDraftBeadsUsesFallbackIDWhenTaskIDMissing(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Missing id task",
		"  @horizon next",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"ai", "draft-beads", "--plan", planPath, "--format", "json"}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected success, got %d stderr=%q", exit, errOut.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode output json: %v output=%q", err, out.String())
	}
	data := payload["data"].(map[string]any)
	suggestions := data["suggestions"].([]any)
	first := suggestions[0].(map[string]any)
	if strings.TrimSpace(first["task_id"].(string)) == "" {
		t.Fatalf("expected non-empty fallback task_id, got %q", first["task_id"])
	}
}
