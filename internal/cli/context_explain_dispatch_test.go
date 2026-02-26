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
