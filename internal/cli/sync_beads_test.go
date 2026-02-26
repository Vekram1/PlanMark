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

func TestSyncBeadsDryRunJSONIncludesPlannedOps(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")
	planBody := strings.Join([]string{
		"- [ ] Task sync dry-run json",
		"  @id fixture.task.sync.dry.json",
		"  @horizon now",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync", "--dry-run", "--plan", planPath, "--state-dir", stateDir, "--format", "json", "beads"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}

	var payload struct {
		Data struct {
			DryRun      bool `json:"dry_run"`
			CreateCount int  `json:"create_count"`
			PlannedOps  []struct {
				Kind string `json:"kind"`
				ID   string `json:"id"`
			} `json:"planned_ops"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json output: %v output=%q", err, out.String())
	}
	if !payload.Data.DryRun {
		t.Fatalf("expected dry_run=true in payload, got %s", out.String())
	}
	if payload.Data.CreateCount != 1 {
		t.Fatalf("expected create_count=1 in dry-run payload, got %s", out.String())
	}
	if len(payload.Data.PlannedOps) != 1 {
		t.Fatalf("expected a single planned op, got %s", out.String())
	}
	if payload.Data.PlannedOps[0].Kind != "create" {
		t.Fatalf("expected create planned op, got %s", out.String())
	}
	if payload.Data.PlannedOps[0].ID != "fixture.task.sync.dry.json" {
		t.Fatalf("expected planned op id fixture.task.sync.dry.json, got %s", out.String())
	}
}

func TestSyncBeadsDryRunTextIncludesPlannedOps(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")
	planBody := strings.Join([]string{
		"- [ ] Task sync dry-run text",
		"  @id fixture.task.sync.dry.text",
		"  @horizon now",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync", "--dry-run", "--plan", planPath, "--state-dir", stateDir, "--format", "text", "beads"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}

	output := out.String()
	if !strings.Contains(output, "planned_ops:\n") {
		t.Fatalf("expected planned_ops block in text output, got %q", output)
	}
	if !strings.Contains(output, "- create fixture.task.sync.dry.text (") {
		t.Fatalf("expected create planned op in text output, got %q", output)
	}
}

func TestSyncBeadsJSONOmitsPlannedOpsWithoutDryRun(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")
	planBody := strings.Join([]string{
		"- [ ] Task sync no dry-run",
		"  @id fixture.task.sync.no_dry_run",
		"  @horizon now",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync", "--plan", planPath, "--state-dir", stateDir, "--format", "json", "beads"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}
	if strings.Contains(out.String(), "\"planned_ops\"") {
		t.Fatalf("expected planned_ops to be omitted without --dry-run, got %q", out.String())
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

func TestSyncBeadsDeletionPolicyFlagParsing(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task sync parse policy",
		"  @id fixture.task.sync.parse_policy",
		"  @horizon now",
		"  @accept cmd:go test ./...",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	for _, policy := range []string{"mark-stale", "close", "detach", "delete"} {
		t.Run(policy, func(t *testing.T) {
			var out bytes.Buffer
			var errOut bytes.Buffer
			exit := Run([]string{"sync", "beads", "--plan", planPath, "--deletion-policy", policy, "--format", "json"}, &out, &errOut)
			if exit != 0 {
				t.Fatalf("expected exit 0 for policy %q, got %d stderr=%q", policy, exit, errOut.String())
			}
			var payload struct {
				Data struct {
					DeletionPolicy string `json:"deletion_policy"`
				} `json:"data"`
			}
			if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
				t.Fatalf("decode json output: %v output=%q", err, out.String())
			}
			if payload.Data.DeletionPolicy != policy {
				t.Fatalf("expected deletion policy %q in output, got %q", policy, payload.Data.DeletionPolicy)
			}
		})
	}
}

func TestSyncBeadsPreservesNoopEntriesAcrossRuns(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")
	planBody := strings.Join([]string{
		"- [ ] Task sync stable",
		"  @id fixture.task.sync.stable",
		"  @horizon now",
		"  @accept cmd:go test ./...",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync", "beads", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected first run exit 0, got %d stderr=%q", exit, errOut.String())
	}

	out.Reset()
	errOut.Reset()
	exit = Run([]string{"sync", "beads", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected second run exit 0, got %d stderr=%q", exit, errOut.String())
	}

	var payload struct {
		Data struct {
			NoopCount    int `json:"noop_count"`
			TasksMutated int `json:"tasks_mutated"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode second run output: %v output=%q", err, out.String())
	}
	if payload.Data.NoopCount == 0 {
		t.Fatalf("expected second run to include noop count, got payload=%s", out.String())
	}
	if payload.Data.TasksMutated != 0 {
		t.Fatalf("expected second run to avoid mutations for unchanged plan, got payload=%s", out.String())
	}

	manifestPath := filepath.Join(stateDir, "sync", "beads-manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest struct {
		Entries []struct {
			ID string `json:"id"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if len(manifest.Entries) != 1 || manifest.Entries[0].ID != "fixture.task.sync.stable" {
		t.Fatalf("expected manifest to preserve existing noop entry, got %s", string(raw))
	}
}
