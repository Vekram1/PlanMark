package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncBeadsUsageRequiresTarget(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync"}, &out, &errOut)
	if exit != 2 {
		t.Fatalf("expected usage exit code 2, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(errOut.String(), "usage: plan sync beads") {
		t.Fatalf("expected usage text, got %q", errOut.String())
	}
}

func TestSyncBeadsWritesManifest(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")
	planBody := strings.Join([]string{
		"- [ ] Task sync",
		"  @id fixture.task.sync",
		"  @horizon now",
		"  @accept cmd:go test ./...",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync", "--plan", planPath, "--state-dir", stateDir, "beads"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}
	manifestPath := filepath.Join(stateDir, "sync", "beads-manifest.json")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("expected manifest at %q: %v", manifestPath, err)
	}
}

func TestSyncBeadsDryRunDoesNotWriteManifest(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")
	planBody := strings.Join([]string{
		"- [ ] Task sync dry-run",
		"  @id fixture.task.sync.dry",
		"  @horizon next",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync", "--dry-run", "--plan", planPath, "--state-dir", stateDir, "beads"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}
	manifestPath := filepath.Join(stateDir, "sync", "beads-manifest.json")
	if _, err := os.Stat(manifestPath); !os.IsNotExist(err) {
		t.Fatalf("expected dry-run to skip manifest write, stat err=%v", err)
	}
}

func TestSyncBeadsAcceptsTargetBeforeFlags(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")
	planBody := strings.Join([]string{
		"- [ ] Task sync positional",
		"  @id fixture.task.sync.positional",
		"  @horizon now",
		"  @accept cmd:go test ./...",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync", "beads", "--plan", planPath, "--state-dir", stateDir}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}
	manifestPath := filepath.Join(stateDir, "sync", "beads-manifest.json")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("expected manifest at %q: %v", manifestPath, err)
	}
}

func TestSyncBeadsDefaultDeletionPolicyMarkStale(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task sync default policy",
		"  @id fixture.task.sync.default_policy",
		"  @horizon now",
		"  @accept cmd:go test ./...",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync", "beads", "--plan", planPath, "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}

	var payload struct {
		Data struct {
			DeletionPolicy string `json:"deletion_policy"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json output: %v", err)
	}
	if payload.Data.DeletionPolicy != "mark-stale" {
		t.Fatalf("expected default deletion policy mark-stale, got %q", payload.Data.DeletionPolicy)
	}
}

func TestSyncBeadsRejectsInvalidDeletionPolicy(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task sync invalid policy",
		"  @id fixture.task.sync.invalid_policy",
		"  @horizon now",
		"  @accept cmd:go test ./...",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync", "beads", "--plan", planPath, "--deletion-policy", "archive"}, &out, &errOut)
	if exit != 2 {
		t.Fatalf("expected usage exit code 2, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(errOut.String(), "invalid deletion policy") {
		t.Fatalf("expected invalid policy message, got %q", errOut.String())
	}
}
