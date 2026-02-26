package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorProfileLooseTextOutput(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task next",
		"  @id task.next",
		"  @horizon next",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"doctor", "--plan", planPath, "--profile", "loose"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(out.String(), "profile: loose") {
		t.Fatalf("expected profile in output, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "parsed_nodes:") || !strings.Contains(out.String(), "parsed_tasks:") {
		t.Fatalf("expected parsed counts in output, got: %q", out.String())
	}
}

func TestDoctorInvalidProfile(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	if err := os.WriteFile(planPath, []byte("- [ ] Task\n  @id task.one\n"), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"doctor", "--plan", planPath, "--profile", "strictest"}, &out, &errOut)
	if exit != 2 {
		t.Fatalf("expected exit 2, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(errOut.String(), "invalid profile") {
		t.Fatalf("expected invalid profile error, got: %q", errOut.String())
	}
}

func TestDoctorReturnsOneOnErrorDiagnostics(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task now",
		"  @id task.now",
		"  @horizon now",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"doctor", "--plan", planPath, "--profile", "exec"}, &out, &errOut)
	if exit != 1 {
		t.Fatalf("expected exit 1 for error diagnostics, got %d stderr=%q output=%q", exit, errOut.String(), out.String())
	}
	if !strings.Contains(out.String(), "MISSING_ACCEPT") {
		t.Fatalf("expected MISSING_ACCEPT diagnostic in output, got: %q", out.String())
	}
}

func TestDoctorLooseProfileDowngradesNowMissingAcceptToWarning(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task now",
		"  @id task.now",
		"  @horizon now",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"doctor", "--plan", planPath, "--profile", "loose"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0 in loose profile, got %d stderr=%q output=%q", exit, errOut.String(), out.String())
	}
	if !strings.Contains(out.String(), "warning MISSING_ACCEPT") {
		t.Fatalf("expected warning diagnostic in output, got: %q", out.String())
	}
}

func TestDoctorJSONOutputUsesProtocolEnvelope(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task next",
		"  @id task.next",
		"  @horizon next",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"doctor", "--plan", planPath, "--profile", "loose", "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json output: %v output=%q", err, out.String())
	}
	if payload["schema_version"] != "v0.1" {
		t.Fatalf("expected schema_version v0.1, got %v", payload["schema_version"])
	}
	if payload["command"] != "doctor" {
		t.Fatalf("expected command doctor, got %v", payload["command"])
	}
	if payload["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", payload["status"])
	}
	if _, ok := payload["data"].(map[string]any); !ok {
		t.Fatalf("expected object data payload, got %T", payload["data"])
	}
}
