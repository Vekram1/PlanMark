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

func TestAISuggestAcceptUnknownDependencyIncludesDoctorSuggestion(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task now",
		"  @id fixture.task.now",
		"  @horizon now",
		"  @deps fixture.task.missing",
		"  @accept cmd:echo ok",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"ai", "suggest-accept", "fixture.task.now", "--plan", planPath, "--format", "json"}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected success, got %d stderr=%q output=%q", exit, errOut.String(), out.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode output json: %v output=%q", err, out.String())
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object data payload, got %T", payload["data"])
	}
	suggestions, ok := data["suggestions"].([]any)
	if !ok {
		t.Fatalf("expected suggestions array, got %T", data["suggestions"])
	}
	found := false
	for _, item := range suggestions {
		if s, ok := item.(string); ok && s == "@accept cmd:plan doctor --plan <path> --profile exec" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected unknown-dependency doctor suggestion, got %v", suggestions)
	}
}

func TestAIUnknownSubcommandReturnsUsageError(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"ai", "unknown-subcommand"}, &out, &errOut)
	if exit != protocol.ExitUsageError {
		t.Fatalf("expected usage error, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(errOut.String(), "unknown ai command") {
		t.Fatalf("expected unknown command message, got %q", errOut.String())
	}
}

func TestAISuggestFixDeterministic(t *testing.T) {
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

	runOnce := func() (string, string, int) {
		var out bytes.Buffer
		var errOut bytes.Buffer
		exit := Run([]string{"ai", "suggest-fix", "--plan", planPath, "--format", "json"}, &out, &errOut)
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
		t.Fatalf("decode json output: %v output=%q", err, out1)
	}
	if payload["command"] != "ai suggest-fix" {
		t.Fatalf("expected command=ai suggest-fix, got %v", payload["command"])
	}
	data := payload["data"].(map[string]any)
	prompt := data["prompt"].(string)
	if !strings.Contains(prompt, "MISSING_ACCEPT") {
		t.Fatalf("expected prompt to include diagnostic code reference, got %q", prompt)
	}
	repairs, ok := data["repairs"].([]any)
	if !ok || len(repairs) == 0 {
		t.Fatalf("expected non-empty repairs, got %v", data["repairs"])
	}
	first := repairs[0].(map[string]any)
	if strings.TrimSpace(first["code"].(string)) == "" {
		t.Fatalf("expected repair code, got %v", first["code"])
	}
}

func TestAISuggestFixInvalidProfileReturnsUsageError(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"ai", "suggest-fix", "--plan", "PLAN.md", "--profile", "strict"}, &out, &errOut)
	if exit != protocol.ExitUsageError {
		t.Fatalf("expected usage error, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(errOut.String(), "invalid profile") {
		t.Fatalf("expected invalid profile error, got %q", errOut.String())
	}
}

func TestAIApplyFixRequiresExplicitApproval(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := "- [ ] Task now\n  @id fixture.task.now\n  @horizon now\n"
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"ai", "apply-fix", "--plan", planPath}, &out, &errOut)
	if exit != protocol.ExitValidationFailed {
		t.Fatalf("expected validation failure, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(errOut.String(), "--approve") {
		t.Fatalf("expected explicit approval requirement, got %q", errOut.String())
	}
}

func TestAIApplyFixJSONOutputIncludesReviewableProposal(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := "- [ ] Task now\n  @id fixture.task.now\n  @horizon now\n"
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"ai", "apply-fix", "--plan", planPath, "--approve", "--format", "json"}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected success, got %d stderr=%q", exit, errOut.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode output json: %v output=%q", err, out.String())
	}
	if payload["command"] != "ai apply-fix" {
		t.Fatalf("expected command field, got %v", payload["command"])
	}
	data := payload["data"].(map[string]any)
	if approved, ok := data["approved"].(bool); !ok || !approved {
		t.Fatalf("expected approved=true, got %v", data["approved"])
	}
	proposal := data["proposal"].(map[string]any)
	if strings.TrimSpace(proposal["base_plan_hash"].(string)) == "" {
		t.Fatalf("expected non-empty base_plan_hash")
	}
	if proposal["proposal_type"] != "plan_delta_preview" {
		t.Fatalf("expected proposal type plan_delta_preview, got %v", proposal["proposal_type"])
	}
}
