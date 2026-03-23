package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/journal"
	"github.com/vikramoddiraju/planmark/internal/tracker"
)

type captureBeadsAdapter struct {
	pushed       []tracker.TaskProjection
	manifestPath string
}

func (a *captureBeadsAdapter) SeedFromSyncManifest(_ tracker.BeadsSyncManifest) {}

func (a *captureBeadsAdapter) PushTask(_ context.Context, task tracker.TaskProjection) (tracker.PushResult, error) {
	a.pushed = append(a.pushed, task)
	return tracker.PushResult{
		RemoteID: "beads:" + task.ID,
		Mutated:  true,
	}, nil
}

func (a *captureBeadsAdapter) WriteSyncManifest(stateDir string) (string, error) {
	if a.manifestPath == "" {
		a.manifestPath = filepath.Join(stateDir, "sync", "beads-manifest.json")
	}
	return a.manifestPath, nil
}

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

func TestSyncBeadsProjectsRicherSemanticTaskFields(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")
	planBody := strings.Join([]string{
		"## Add migration",
		"@id api.migrate",
		"@horizon now",
		"@deps api.schema, api.runtime",
		"@accept cmd:go test ./...",
		"",
		"- [ ] Write additive migration",
		"- [x] Verify rollback",
		"",
		"### Notes",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	restore := newBeadsSyncAdapter
	adapter := &captureBeadsAdapter{}
	newBeadsSyncAdapter = func() beadsSyncAdapter { return adapter }
	defer func() { newBeadsSyncAdapter = restore }()

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync", "beads", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}
	if len(adapter.pushed) != 1 {
		t.Fatalf("expected one pushed task projection, got %#v", adapter.pushed)
	}

	got := adapter.pushed[0]
	if got.ID != "api.migrate" || got.Title != "Add migration" {
		t.Fatalf("unexpected projected identity/title: %#v", got)
	}
	if got.Horizon != "now" {
		t.Fatalf("expected horizon now, got %#v", got)
	}
	if !reflect.DeepEqual(got.Dependencies, []string{"api.schema", "api.runtime"}) {
		t.Fatalf("unexpected deps: %#v", got.Dependencies)
	}
	if len(got.Steps) != 2 {
		t.Fatalf("expected two projected steps, got %#v", got.Steps)
	}
	if got.Steps[0].Title != "Write additive migration" || got.Steps[1].Title != "Verify rollback" || !got.Steps[1].Checked {
		t.Fatalf("unexpected projected steps: %#v", got.Steps)
	}
	if len(got.Evidence) != 1 {
		t.Fatalf("expected one projected evidence node ref, got %#v", got.Evidence)
	}
	if got.Provenance.NodeRef == "" || got.Provenance.Path == "" || got.Provenance.SourceHash == "" {
		t.Fatalf("expected populated provenance, got %#v", got.Provenance)
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

func TestSyncBeadsCLIJSONOutputStable(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")
	planBody := strings.Join([]string{
		"- [ ] Task sync stable json",
		"  @id fixture.task.sync.json_stable",
		"  @horizon now",
		"  @accept cmd:go test ./...",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	run := func() map[string]any {
		var out bytes.Buffer
		var errOut bytes.Buffer
		exit := Run([]string{"sync", "beads", "--dry-run", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
		if exit != 0 {
			t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
		}
		var payload map[string]any
		if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
			t.Fatalf("decode json output: %v output=%q", err, out.String())
		}
		return payload
	}

	first := run()
	second := run()

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("expected stable sync json output across identical runs\nfirst=%v\nsecond=%v", first, second)
	}
	if first["schema_version"] != "v0.1" {
		t.Fatalf("expected schema_version v0.1, got %v", first["schema_version"])
	}
	if first["command"] != "sync beads" {
		t.Fatalf("expected command sync beads, got %v", first["command"])
	}
	if first["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", first["status"])
	}
}

func TestBeadProvenanceMismatchMarkedStale(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")

	initialPlan := strings.Join([]string{
		"- [ ] Provenance gate task",
		"  @id fixture.task.provenance",
		"  @horizon now",
		"  @accept cmd:go test ./...",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(initialPlan), 0o644); err != nil {
		t.Fatalf("write initial plan: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	firstExit := Run([]string{"sync", "beads", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
	if firstExit != 0 {
		t.Fatalf("expected initial sync success, got %d stderr=%q", firstExit, errOut.String())
	}

	changedPlan := strings.Join([]string{
		"## moved section",
		"",
		"- [ ] Provenance gate task",
		"  @id fixture.task.provenance",
		"  @horizon now",
		"  @accept cmd:go test ./...",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(changedPlan), 0o644); err != nil {
		t.Fatalf("write changed plan: %v", err)
	}

	out.Reset()
	errOut.Reset()
	secondExit := Run([]string{"sync", "beads", "--dry-run", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
	if secondExit != 0 {
		t.Fatalf("expected dry-run success, got %d stderr=%q", secondExit, errOut.String())
	}

	var payload struct {
		Data struct {
			PlannedOps []struct {
				Kind   string `json:"kind"`
				ID     string `json:"id"`
				Reason string `json:"reason"`
			} `json:"planned_ops"`
			MarkStaleCount int `json:"mark_stale_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode dry-run json: %v output=%q", err, out.String())
	}
	if payload.Data.MarkStaleCount != 1 {
		t.Fatalf("expected mark_stale_count=1, got %d output=%q", payload.Data.MarkStaleCount, out.String())
	}
	if len(payload.Data.PlannedOps) != 1 {
		t.Fatalf("expected one planned op, got %d output=%q", len(payload.Data.PlannedOps), out.String())
	}
	op := payload.Data.PlannedOps[0]
	if op.Kind != "mark-stale" || op.ID != "fixture.task.provenance" {
		t.Fatalf("expected mark-stale op for fixture.task.provenance, got %+v", op)
	}
	if !strings.Contains(op.Reason, "stale provenance mismatch") {
		t.Fatalf("expected explicit stale provenance reason, got %q", op.Reason)
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

func TestBeadsSyncRetryTransientFailure(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")
	planBody := strings.Join([]string{
		"- [ ] Task sync retry transient",
		"  @id fixture.task.sync.retry.transient",
		"  @horizon now",
		"  @accept cmd:go test ./...",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	restore := newBeadsSyncAdapter
	adapter := tracker.NewBeadsAdapter()
	adapter.SetPushFailures("fixture.task.sync.retry.transient", []error{
		fmt.Errorf("%w: temporary gateway timeout", tracker.ErrTransientSync),
	})
	newBeadsSyncAdapter = func() beadsSyncAdapter { return adapter }
	defer func() { newBeadsSyncAdapter = restore }()

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync", "beads", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0 after transient retry, got %d stderr=%q", exit, errOut.String())
	}

	j, err := journal.Load(stateDir)
	if err != nil {
		t.Fatalf("load sync journal: %v", err)
	}
	failed := 0
	success := 0
	for _, a := range j.Attempts {
		if a.ID != "fixture.task.sync.retry.transient" {
			continue
		}
		if a.Outcome == journal.OutcomeFailed {
			failed++
		}
		if a.Outcome == journal.OutcomeSuccess {
			success++
		}
	}
	if failed < 1 || success < 1 {
		t.Fatalf("expected retry journal history with failed then success attempts, got %#v", j.Attempts)
	}
}

func TestBeadsSyncRateLimitBackoffBehavior(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")
	planBody := strings.Join([]string{
		"- [ ] Task sync retry rate-limit",
		"  @id fixture.task.sync.retry.ratelimit",
		"  @horizon now",
		"  @accept cmd:go test ./...",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	restore := newBeadsSyncAdapter
	adapter := tracker.NewBeadsAdapter()
	adapter.SetPushFailures("fixture.task.sync.retry.ratelimit", []error{
		fmt.Errorf("%w: 429", tracker.ErrRateLimitedSync),
		fmt.Errorf("%w: 429", tracker.ErrRateLimitedSync),
	})
	newBeadsSyncAdapter = func() beadsSyncAdapter { return adapter }
	defer func() { newBeadsSyncAdapter = restore }()

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync", "beads", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0 after rate-limit retries, got %d stderr=%q", exit, errOut.String())
	}

	j, err := journal.Load(stateDir)
	if err != nil {
		t.Fatalf("load sync journal: %v", err)
	}
	var retryAnnotated int
	var successAttempt int
	for _, a := range j.Attempts {
		if a.ID != "fixture.task.sync.retry.ratelimit" {
			continue
		}
		if a.Outcome == journal.OutcomeFailed && strings.Contains(a.Error, "backoff_ms=") {
			retryAnnotated++
		}
		if a.Outcome == journal.OutcomeSuccess {
			successAttempt = a.Attempt
		}
	}
	if retryAnnotated < 2 {
		t.Fatalf("expected failed retry attempts to include deterministic backoff annotation, got %#v", j.Attempts)
	}
	if successAttempt != 3 {
		t.Fatalf("expected success on attempt 3 after two rate-limit retries, got %d", successAttempt)
	}
}
