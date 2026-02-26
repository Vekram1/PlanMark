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

func TestDoctorFixOutDeterministic(t *testing.T) {
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

	outA := filepath.Join(tmp, "delta-a.json")
	outB := filepath.Join(tmp, "delta-b.json")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := Run([]string{"doctor", "--plan", planPath, "--profile", "exec", "--fix-out", outA}, &stdout, &stderr)
	if exit != 1 {
		t.Fatalf("expected validation failure exit, got %d stderr=%q", exit, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	exit = Run([]string{"doctor", "--plan", planPath, "--profile", "exec", "--fix-out", outB}, &stdout, &stderr)
	if exit != 1 {
		t.Fatalf("expected validation failure exit on second run, got %d stderr=%q", exit, stderr.String())
	}

	rawA, err := os.ReadFile(outA)
	if err != nil {
		t.Fatalf("read first fix-out: %v", err)
	}
	rawB, err := os.ReadFile(outB)
	if err != nil {
		t.Fatalf("read second fix-out: %v", err)
	}
	if string(rawA) != string(rawB) {
		t.Fatalf("expected deterministic fix-out bytes\nA:\n%s\nB:\n%s", string(rawA), string(rawB))
	}
	if !strings.Contains(string(rawA), "\"@accept cmd:<command>\"") {
		t.Fatalf("expected missing-accept suggestion in fix-out: %s", string(rawA))
	}
}

func TestDoctorCompileLimitFailureIsDiagnostic(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	line := strings.Repeat("x", 300*1024)
	if err := os.WriteFile(planPath, []byte(line+"\n"), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"doctor", "--plan", planPath, "--format", "text"}, &out, &errOut)
	if exit != 1 {
		t.Fatalf("expected validation failure exit, got %d stderr=%q output=%q", exit, errOut.String(), out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected empty stderr for diagnostic-form compile limits, got %q", errOut.String())
	}
	if !strings.Contains(out.String(), "COMPILE_LIMIT_EXCEEDED") {
		t.Fatalf("expected COMPILE_LIMIT_EXCEEDED diagnostic, got %q", out.String())
	}
}

func TestDoctorInvalidProfilePrecedenceOverCompileLimit(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	line := strings.Repeat("x", 300*1024)
	if err := os.WriteFile(planPath, []byte(line+"\n"), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"doctor", "--plan", planPath, "--profile", "strictest"}, &out, &errOut)
	if exit != 2 {
		t.Fatalf("expected usage error exit for invalid profile, got %d stderr=%q output=%q", exit, errOut.String(), out.String())
	}
	if !strings.Contains(errOut.String(), "invalid profile") {
		t.Fatalf("expected invalid profile error, got %q", errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("expected no stdout for invalid profile, got %q", out.String())
	}
}

func TestRepoConfigProfileOverrides(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(tmp, ".planmark.yaml"), []byte("profiles:\n  doctor: exec\n"), 0o644); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"doctor", "--plan", planPath}, &out, &errOut)
	if exit != 1 {
		t.Fatalf("expected exec profile from config to fail for missing accept, got %d stderr=%q output=%q", exit, errOut.String(), out.String())
	}
	if !strings.Contains(out.String(), "profile: exec") {
		t.Fatalf("expected profile exec from config, got output: %q", out.String())
	}

	out.Reset()
	errOut.Reset()
	exit = Run([]string{"doctor", "--plan", planPath, "--profile", "loose"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected explicit --profile to override config, got %d stderr=%q output=%q", exit, errOut.String(), out.String())
	}
	if !strings.Contains(out.String(), "profile: loose") {
		t.Fatalf("expected profile loose from flag override, got output: %q", out.String())
	}
}

func TestDoctorRichFormatOutput(t *testing.T) {
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
	exit := Run([]string{"doctor", "--plan", planPath, "--profile", "exec", "--format", "rich"}, &out, &errOut)
	if exit != 1 {
		t.Fatalf("expected validation failure exit for missing accept, got %d stderr=%q output=%q", exit, errOut.String(), out.String())
	}
	if !strings.Contains(out.String(), "diagnostics:") || !strings.Contains(out.String(), "entries:") {
		t.Fatalf("expected rich diagnostics sections, got %q", out.String())
	}
	if !strings.Contains(out.String(), "[ERROR] MISSING_ACCEPT") {
		t.Fatalf("expected rich entry with code, got %q", out.String())
	}
}

func TestDoctorFixOutUsesResolvedProfile(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(tmp, ".planmark.yaml"), []byte("profiles:\n  doctor: exec\n"), 0o644); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	fixOutPath := filepath.Join(tmp, "fix.json")
	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"doctor", "--plan", planPath, "--fix-out", fixOutPath}, &out, &errOut)
	if exit != 1 {
		t.Fatalf("expected validation failure exit, got %d stderr=%q output=%q", exit, errOut.String(), out.String())
	}

	raw, err := os.ReadFile(fixOutPath)
	if err != nil {
		t.Fatalf("read fix-out: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode fix-out: %v raw=%q", err, string(raw))
	}
	if payload["profile"] != "exec" {
		t.Fatalf("expected resolved profile exec in fix-out, got %v", payload["profile"])
	}
}
