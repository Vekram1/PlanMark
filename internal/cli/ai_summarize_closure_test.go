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

func TestAISummarizeClosureTextOutput(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Root",
		"  @id fixture.task.root",
		"  @horizon now",
		"  @deps fixture.task.dep",
		"- [ ] Dep",
		"  @id fixture.task.dep",
		"  @horizon next",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"ai", "summarize-closure", "fixture.task.root", "--plan", planPath}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected success, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(out.String(), "closure_count: 1") {
		t.Fatalf("expected closure_count in output, got %q", out.String())
	}
	if !strings.Contains(out.String(), "fixture.task.dep") {
		t.Fatalf("expected dep summary in output, got %q", out.String())
	}
}

func TestAISummarizeClosureJSONOutput(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Root",
		"  @id fixture.task.root",
		"  @horizon now",
		"  @deps fixture.task.dep",
		"- [ ] Dep",
		"  @id fixture.task.dep",
		"  @horizon next",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"ai", "summarize-closure", "fixture.task.root", "--plan", planPath, "--format", "json"}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected success, got %d stderr=%q", exit, errOut.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json output: %v output=%q", err, out.String())
	}
	if payload["command"] != "ai summarize-closure" {
		t.Fatalf("expected command field, got %v", payload["command"])
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object data payload, got %T", payload["data"])
	}
	if count, _ := data["closure_count"].(float64); int(count) != 1 {
		t.Fatalf("expected closure_count=1, got %v", data["closure_count"])
	}
	closure, ok := data["closure"].([]any)
	if !ok || len(closure) != 1 {
		t.Fatalf("expected single closure entry, got %v", data["closure"])
	}
}

func TestAISummarizeClosureMissingTask(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] One",
		"  @id fixture.task.one",
		"  @horizon next",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"ai", "summarize-closure", "fixture.task.missing", "--plan", planPath}, &out, &errOut)
	if exit != protocol.ExitValidationFailed {
		t.Fatalf("expected validation failure, got %d stderr=%q output=%q", exit, errOut.String(), out.String())
	}
	if !strings.Contains(errOut.String(), "task not found") {
		t.Fatalf("expected task-not-found error, got %q", errOut.String())
	}
}

func TestAISummarizeClosureUnresolvedDependency(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Root",
		"  @id fixture.task.root",
		"  @horizon now",
		"  @deps fixture.task.missing_dep",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"ai", "summarize-closure", "fixture.task.root", "--plan", planPath}, &out, &errOut)
	if exit != protocol.ExitValidationFailed {
		t.Fatalf("expected validation failure, got %d stderr=%q output=%q", exit, errOut.String(), out.String())
	}
	if !strings.Contains(errOut.String(), "dependency task not found") {
		t.Fatalf("expected unresolved dependency error, got %q", errOut.String())
	}
}
