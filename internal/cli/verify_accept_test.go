package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/accept"
	"github.com/vikramoddiraju/planmark/internal/protocol"
)

func TestVerifyAcceptJSONReceiptOK(t *testing.T) {
	planPath := filepath.Join(t.TempDir(), "PLAN.md")
	planBody := "- [ ] Run a command\n  @id fixture.verify.ok\n  @accept cmd:printf ok\n"
	if err := osWriteFile(planPath, []byte(planBody)); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runVerifyAccept([]string{"fixture.verify.ok", "--plan", planPath, "--format", "json", "--timeout-ms", "5000"}, &stdout, &stderr)
	if code != protocol.ExitSuccess {
		t.Fatalf("expected success exit code, got %d stderr=%q", code, stderr.String())
	}

	var env protocol.Envelope[accept.VerificationReceipt]
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode receipt envelope: %v", err)
	}
	if env.Command != "verify-accept" {
		t.Fatalf("unexpected command %q", env.Command)
	}
	if env.Data.SchemaVersion != accept.ReceiptSchemaVersion {
		t.Fatalf("unexpected receipt schema version %q", env.Data.SchemaVersion)
	}
	if env.Data.Result.Status != "ok" {
		t.Fatalf("unexpected result status %q", env.Data.Result.Status)
	}
	if env.Data.Command.CommandText != "printf ok" {
		t.Fatalf("unexpected command text %q", env.Data.Command.CommandText)
	}
}

func TestVerifyAcceptMissingTaskReturnsValidationFailure(t *testing.T) {
	planPath := filepath.Join(t.TempDir(), "PLAN.md")
	planBody := "- [ ] Task\n  @id fixture.verify.exists\n  @accept cmd:printf ok\n"
	if err := osWriteFile(planPath, []byte(planBody)); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runVerifyAccept([]string{"fixture.verify.missing", "--plan", planPath}, &stdout, &stderr)
	if code != protocol.ExitValidationFailed {
		t.Fatalf("expected validation failure, got %d", code)
	}
}

func TestRunDispatchesVerifyAccept(t *testing.T) {
	planPath := filepath.Join(t.TempDir(), "PLAN.md")
	planBody := "- [ ] Run a command\n  @id fixture.verify.dispatch\n  @accept cmd:printf ok\n"
	if err := osWriteFile(planPath, []byte(planBody)); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{"verify-accept", "fixture.verify.dispatch", "--plan", planPath, "--format", "json"}, &stdout, &stderr)
	if code != protocol.ExitSuccess {
		t.Fatalf("expected success exit code, got %d stderr=%q", code, stderr.String())
	}
}

func TestVerifyAcceptUsesRepoRootWhenGitMarkerExists(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("create git marker: %v", err)
	}
	planDir := filepath.Join(root, "plans")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("create plan dir: %v", err)
	}
	planPath := filepath.Join(planDir, "PLAN.md")
	planBody := "- [ ] Write marker\n  @id fixture.verify.cwd\n  @accept cmd:touch marker.txt\n"
	if err := osWriteFile(planPath, []byte(planBody)); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runVerifyAccept([]string{"fixture.verify.cwd", "--plan", planPath, "--format", "json"}, &stdout, &stderr)
	if code != protocol.ExitSuccess {
		t.Fatalf("expected success exit code, got %d stderr=%q", code, stderr.String())
	}

	if _, err := os.Stat(filepath.Join(root, "marker.txt")); err != nil {
		t.Fatalf("expected marker at repo root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(planDir, "marker.txt")); !os.IsNotExist(err) {
		t.Fatalf("did not expect marker in plan directory")
	}
}

func osWriteFile(path string, payload []byte) error {
	return os.WriteFile(path, payload, 0o644)
}
