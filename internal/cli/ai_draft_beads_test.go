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
	if !ok || len(suggestions) < 1 {
		t.Fatalf("expected at least one suggestion, got %v", data["suggestions"])
	}
	first := suggestions[0].(map[string]any)
	if first["task_id"] != "fixture.task.now" {
		t.Fatalf("expected now task suggestion, got %v", first["task_id"])
	}
	if first["draft_level"] != "parent" {
		t.Fatalf("expected parent draft first, got %v", first["draft_level"])
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

func TestAIDraftBeadsGeneratesChildDraftsDeterministically(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Parent task",
		"  @id fixture.task.parent",
		"  @horizon next",
		"  @accept cmd:echo one",
		"  @accept cmd:echo two",
		"  @deps fixture.dep.a,fixture.dep.b",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	runOnce := func() (string, string, int) {
		var out bytes.Buffer
		var errOut bytes.Buffer
		exit := Run([]string{"ai", "draft-beads", "--plan", planPath, "--format", "json", "--limit", "8"}, &out, &errOut)
		return out.String(), errOut.String(), exit
	}

	out1, err1, exit1 := runOnce()
	out2, err2, exit2 := runOnce()
	if exit1 != protocol.ExitSuccess || exit2 != protocol.ExitSuccess {
		t.Fatalf("expected success exits, got %d/%d stderr1=%q stderr2=%q", exit1, exit2, err1, err2)
	}
	if out1 != out2 {
		t.Fatalf("expected deterministic output\nfirst=%s\nsecond=%s", out1, out2)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(out1), &payload); err != nil {
		t.Fatalf("decode output json: %v output=%q", err, out1)
	}
	data := payload["data"].(map[string]any)
	suggestions := data["suggestions"].([]any)
	if len(suggestions) < 3 {
		t.Fatalf("expected parent + child suggestions, got %d", len(suggestions))
	}
	first := suggestions[0].(map[string]any)
	if first["draft_level"] != "parent" {
		t.Fatalf("expected first suggestion to be parent, got %v", first["draft_level"])
	}
	second := suggestions[1].(map[string]any)
	if second["draft_level"] != "child" {
		t.Fatalf("expected second suggestion to be child, got %v", second["draft_level"])
	}
	if second["parent_task_id"] != "fixture.task.parent" {
		t.Fatalf("expected parent_task_id fixture.task.parent, got %v", second["parent_task_id"])
	}
	if idx, ok := second["child_order_index"].(float64); !ok || int(idx) != 1 {
		t.Fatalf("expected child_order_index=1, got %v", second["child_order_index"])
	}
	third := suggestions[2].(map[string]any)
	if idx, ok := third["child_order_index"].(float64); !ok || int(idx) != 2 {
		t.Fatalf("expected child_order_index=2, got %v", third["child_order_index"])
	}
}

func TestAIDraftBeadsChildDepsUseStableSortedOrder(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Parent task",
		"  @id fixture.task.parent",
		"  @horizon next",
		"  @deps fixture.dep.z,fixture.dep.a",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"ai", "draft-beads", "--plan", planPath, "--format", "json", "--limit", "5"}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected success, got %d stderr=%q", exit, errOut.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode output json: %v output=%q", err, out.String())
	}
	data := payload["data"].(map[string]any)
	suggestions := data["suggestions"].([]any)
	if len(suggestions) < 3 {
		t.Fatalf("expected parent + dep children, got %d", len(suggestions))
	}
	depChild := suggestions[1].(map[string]any)
	body := depChild["suggested_body"].(string)
	if !strings.Contains(body, "Dependency to align: fixture.dep.a") {
		t.Fatalf("expected first dependency child to use sorted dep fixture.dep.a, got %q", body)
	}
}
