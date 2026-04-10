package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/config"
	"github.com/vikramoddiraju/planmark/internal/diag"
	"github.com/vikramoddiraju/planmark/internal/journal"
	"github.com/vikramoddiraju/planmark/internal/protocol"
	"github.com/vikramoddiraju/planmark/internal/tracker"
)

type captureBeadsAdapter struct {
	pushed            []tracker.TaskProjection
	dependencySynced  []tracker.TaskProjection
	staled            []string
	staleReasons      map[string]string
	staleCandidates   []tracker.SyncManifestEntry
	cleanupCandidates []tracker.CleanupCandidate
	cleaned           []tracker.CleanupCandidate
	closedIDs         map[string]bool
	manifestPath      string
	profile           tracker.RenderProfile
	reconcileFn       func(manifest tracker.SyncManifest) tracker.SyncManifest
	projectionByID    map[string]string
	provenanceByID    map[string]tracker.TaskProvenance
	sourceHashByID    map[string]string
	pushFailuresByID  map[string][]error
}

func (a *captureBeadsAdapter) SeedFromSyncManifest(manifest tracker.SyncManifest) {
	if a.projectionByID == nil {
		a.projectionByID = map[string]string{}
	}
	if a.sourceHashByID == nil {
		a.sourceHashByID = map[string]string{}
	}
	if a.provenanceByID == nil {
		a.provenanceByID = map[string]tracker.TaskProvenance{}
	}
	for _, entry := range manifest.Entries {
		if strings.TrimSpace(entry.ID) == "" {
			continue
		}
		a.projectionByID[entry.ID] = strings.TrimSpace(entry.ProjectionHash)
		a.sourceHashByID[entry.ID] = strings.TrimSpace(entry.SourceHash)
		a.provenanceByID[entry.ID] = tracker.TaskProvenance{
			NodeRef:    strings.TrimSpace(entry.NodeRef),
			Path:       strings.TrimSpace(entry.SourcePath),
			StartLine:  entry.SourceStartLine,
			EndLine:    entry.SourceEndLine,
			SourceHash: strings.TrimSpace(entry.SourceHash),
			CompileID:  strings.TrimSpace(entry.CompileID),
		}
	}
}
func (a *captureBeadsAdapter) SetRenderProfile(profile tracker.RenderProfile) { a.profile = profile }
func (a *captureBeadsAdapter) ValidateTask(_ tracker.TaskProjection) error    { return nil }
func (a *captureBeadsAdapter) ReconcileSyncManifest(_ context.Context, manifest tracker.SyncManifest) (tracker.SyncManifest, error) {
	if a.reconcileFn != nil {
		return a.reconcileFn(manifest), nil
	}
	return manifest, nil
}
func (a *captureBeadsAdapter) Capabilities() tracker.TrackerCapabilities {
	return tracker.NewBeadsAdapter().Capabilities()
}
func (a *captureBeadsAdapter) ListStaleCandidates(_ context.Context, desiredIDs map[string]struct{}) ([]tracker.SyncManifestEntry, error) {
	out := make([]tracker.SyncManifestEntry, 0, len(a.staleCandidates))
	for _, entry := range a.staleCandidates {
		if strings.TrimSpace(entry.ID) == "" {
			continue
		}
		if _, desired := desiredIDs[entry.ID]; desired {
			continue
		}
		out = append(out, entry)
	}
	return out, nil
}

func (a *captureBeadsAdapter) PushTask(_ context.Context, task tracker.TaskProjection) (tracker.PushResult, error) {
	if queued := a.pushFailuresByID[task.ID]; len(queued) > 0 {
		err := queued[0]
		if len(queued) == 1 {
			delete(a.pushFailuresByID, task.ID)
		} else {
			a.pushFailuresByID[task.ID] = queued[1:]
		}
		return tracker.PushResult{}, err
	}
	if a.projectionByID == nil {
		a.projectionByID = map[string]string{}
	}
	if a.sourceHashByID == nil {
		a.sourceHashByID = map[string]string{}
	}
	if a.provenanceByID == nil {
		a.provenanceByID = map[string]tracker.TaskProvenance{}
	}
	hash, err := tracker.TaskProjectionHash(task)
	if err != nil {
		return tracker.PushResult{}, err
	}
	a.pushed = append(a.pushed, task)
	if prior, ok := a.projectionByID[task.ID]; ok && prior == hash {
		if canonicalTaskStatusForTest(task.CanonicalStatus) == "done" {
			if a.closedIDs != nil && a.closedIDs[task.ID] {
				return tracker.PushResult{
					RemoteID:   "bead:" + task.ID,
					Noop:       true,
					Mutated:    false,
					Diagnostic: "projection unchanged",
				}, nil
			}
			if a.closedIDs != nil {
				a.closedIDs[task.ID] = true
			}
			return tracker.PushResult{
				RemoteID:   "bead:" + task.ID,
				Noop:       false,
				Mutated:    true,
				Diagnostic: "closed tracker issue for canonically completed task",
			}, nil
		}
		if a.closedIDs != nil && a.closedIDs[task.ID] {
			delete(a.closedIDs, task.ID)
			return tracker.PushResult{
				RemoteID:   "bead:" + task.ID,
				Noop:       false,
				Mutated:    true,
				Diagnostic: "reopened closed tracker issue with unchanged projection",
			}, nil
		}
		return tracker.PushResult{
			RemoteID:   "bead:" + task.ID,
			Noop:       true,
			Mutated:    false,
			Diagnostic: "projection unchanged",
		}, nil
	}
	diagnostic := "projection updated"
	if priorSource, ok := a.sourceHashByID[task.ID]; ok && priorSource != strings.TrimSpace(task.Provenance.SourceHash) {
		diagnostic = "projection drift detected: source hash changed"
	}
	a.projectionByID[task.ID] = hash
	a.sourceHashByID[task.ID] = strings.TrimSpace(task.Provenance.SourceHash)
	a.provenanceByID[task.ID] = task.Provenance
	if a.closedIDs != nil {
		a.closedIDs[task.ID] = canonicalTaskStatusForTest(task.CanonicalStatus) == "done"
	}
	return tracker.PushResult{
		RemoteID:   "bead:" + task.ID,
		Mutated:    true,
		Diagnostic: diagnostic,
	}, nil
}

func (a *captureBeadsAdapter) SyncDependencies(_ context.Context, tasks map[string]tracker.TaskProjection) error {
	ids := make([]string, 0, len(tasks))
	for id := range tasks {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	a.dependencySynced = a.dependencySynced[:0]
	for _, id := range ids {
		a.dependencySynced = append(a.dependencySynced, tasks[id])
	}
	return nil
}

func (a *captureBeadsAdapter) MarkTaskStale(_ context.Context, id string, reason string) (tracker.PushResult, error) {
	a.staled = append(a.staled, id)
	if a.staleReasons == nil {
		a.staleReasons = map[string]string{}
	}
	a.staleReasons[id] = reason
	delete(a.projectionByID, id)
	delete(a.sourceHashByID, id)
	delete(a.provenanceByID, id)
	return tracker.PushResult{
		RemoteID:   "bead:" + id,
		Mutated:    true,
		Diagnostic: "tracker issue closed for stale task",
	}, nil
}

func (a *captureBeadsAdapter) WriteSyncManifest(stateDir string) (string, error) {
	if a.manifestPath == "" {
		a.manifestPath = filepath.Join(stateDir, "sync", "beads-manifest.json")
	}
	if err := os.MkdirAll(filepath.Dir(a.manifestPath), 0o755); err != nil {
		return "", err
	}
	entries := make([]tracker.SyncManifestEntry, 0, len(a.projectionByID))
	for id, projectionHash := range a.projectionByID {
		provenance := a.provenanceByID[id]
		entries = append(entries, tracker.SyncManifestEntry{
			ID:              id,
			RemoteID:        "bead:" + id,
			ProjectionHash:  projectionHash,
			NodeRef:         provenance.NodeRef,
			SourcePath:      provenance.Path,
			SourceStartLine: provenance.StartLine,
			SourceEndLine:   provenance.EndLine,
			SourceHash:      a.sourceHashByID[id],
			CompileID:       provenance.CompileID,
		})
	}
	manifest := tracker.SyncManifest{
		SchemaVersion: tracker.SyncManifestSchemaVersionV01,
		Entries:       entries,
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := os.WriteFile(a.manifestPath, data, 0o644); err != nil {
		return "", err
	}
	return a.manifestPath, nil
}

func installCaptureBeadsAdapter(t *testing.T) *captureBeadsAdapter {
	t.Helper()
	restore := newBeadsSyncAdapter
	adapter := &captureBeadsAdapter{
		projectionByID:   map[string]string{},
		provenanceByID:   map[string]tracker.TaskProvenance{},
		sourceHashByID:   map[string]string{},
		pushFailuresByID: map[string][]error{},
		staleReasons:     map[string]string{},
		closedIDs:        map[string]bool{},
	}
	newBeadsSyncAdapter = func() beadsSyncAdapter { return adapter }
	t.Cleanup(func() {
		newBeadsSyncAdapter = restore
	})
	return adapter
}

func canonicalTaskStatusForTest(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "done":
		return "done"
	default:
		return "open"
	}
}

func (a *captureBeadsAdapter) SetPushFailures(id string, failures []error) {
	if a.pushFailuresByID == nil {
		a.pushFailuresByID = map[string][]error{}
	}
	if len(failures) == 0 {
		delete(a.pushFailuresByID, id)
		return
	}
	copied := make([]error, len(failures))
	copy(copied, failures)
	a.pushFailuresByID[id] = copied
}

func TestSyncBeadsUsageRequiresTarget(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync"}, &out, &errOut)
	if exit != 2 {
		t.Fatalf("expected usage exit code 2, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(errOut.String(), "missing --plan") {
		t.Fatalf("expected usage text, got %q", errOut.String())
	}
}

func TestSyncBeadsDefaultsToBeadsAdapter(t *testing.T) {
	installCaptureBeadsAdapter(t)
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")
	planBody := strings.Join([]string{
		"- [ ] Task sync default adapter",
		"  @id fixture.task.sync.default_adapter",
		"  @horizon now",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(out.String(), "\"target\":\"beads\"") {
		t.Fatalf("expected default beads target in json output, got %q", out.String())
	}
}

func TestSyncBeadsWritesManifest(t *testing.T) {
	installCaptureBeadsAdapter(t)
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
		"## Schema task",
		"@id api.schema",
		"@horizon later",
		"",
		"## Runtime task",
		"@id api.runtime",
		"@horizon later",
		"",
		"## Add migration",
		"@id api.migrate",
		"@horizon now",
		"@deps api.schema, api.runtime",
		"@accept cmd:go test ./...",
		"",
		"We need additive rollout first.",
		"",
		"- [ ] Write additive migration",
		"- [x] Verify rollback",
		"",
		"### Notes",
		"Prefer expand-contract over a table rewrite.",
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
	if len(adapter.pushed) != 3 {
		t.Fatalf("expected three pushed task projections, got %#v", adapter.pushed)
	}

	var got tracker.TaskProjection
	for _, projection := range adapter.pushed {
		if projection.ID == "api.migrate" {
			got = projection
			break
		}
	}
	if got.ID == "" {
		t.Fatalf("expected api.migrate projection in pushed set, got %#v", adapter.pushed)
	}
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
	if len(got.Sections) != 1 {
		t.Fatalf("expected one projected section, got %#v", got.Sections)
	}
	if got.Sections[0].Key != "details" || got.Sections[0].Title != "Details" {
		t.Fatalf("unexpected projected section metadata: %#v", got.Sections)
	}
	if !reflect.DeepEqual(got.Sections[0].Body, []string{
		"We need additive rollout first.",
		"",
		"### Notes",
		"Prefer expand-contract over a table rewrite.",
	}) {
		t.Fatalf("unexpected projected section body: %#v", got.Sections[0].Body)
	}
	if len(got.Evidence) != 1 {
		t.Fatalf("expected one projected evidence node ref, got %#v", got.Evidence)
	}
	if got.Provenance.NodeRef == "" || got.Provenance.Path == "" || got.Provenance.SourceHash == "" {
		t.Fatalf("expected populated provenance, got %#v", got.Provenance)
	}
}

func TestSyncBeadsReconcilesNativeDependenciesAfterPush(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")
	planBody := strings.Join([]string{
		"- [ ] Root task",
		"  @id fixture.task.root",
		"  @deps fixture.task.dep",
		"",
		"- [ ] Dependency task",
		"  @id fixture.task.dep",
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
	if len(adapter.pushed) != 2 {
		t.Fatalf("expected two pushed tasks, got %#v", adapter.pushed)
	}
	if len(adapter.dependencySynced) != 2 {
		t.Fatalf("expected dependency sync for both tasks, got %#v", adapter.dependencySynced)
	}
	if adapter.dependencySynced[0].ID != "fixture.task.dep" || adapter.dependencySynced[1].ID != "fixture.task.root" {
		t.Fatalf("expected dependency sync to be deterministic by task id, got %#v", adapter.dependencySynced)
	}
	if !reflect.DeepEqual(adapter.dependencySynced[1].Dependencies, []string{"fixture.task.dep"}) {
		t.Fatalf("expected dependency sync payload to retain deps, got %#v", adapter.dependencySynced[1])
	}
}

func TestSyncUsesLinearAdapterAndWritesManifest(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")
	configPath := filepath.Join(tmp, ".planmark.yaml")
	planBody := strings.Join([]string{
		"- [ ] Task sync linear config",
		"  @id fixture.task.sync.linear",
		"  @horizon now",
		"  @accept cmd:go test ./...",
	}, "\n")
	configBody := strings.Join([]string{
		"schema_version: v0.1",
		"tracker:",
		"  adapter: linear",
		"  profile: compact",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(out.String(), "\"target\":\"linear\"") {
		t.Fatalf("expected linear target in json output, got %q", out.String())
	}
	if !strings.Contains(out.String(), "\"command\":\"sync linear\"") {
		t.Fatalf("expected sync linear command in json output, got %q", out.String())
	}
	manifestPath := filepath.Join(stateDir, "sync", "linear-manifest.json")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("expected linear manifest at %q: %v", manifestPath, err)
	}
}

func TestSyncCLIProfileOverridesConfigProfile(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")
	configPath := filepath.Join(tmp, ".planmark.yaml")
	planBody := strings.Join([]string{
		"## Add migration",
		"@id api.migrate",
		"@horizon now",
		"@accept cmd:go test ./...",
		"",
		"- [ ] Write additive migration",
	}, "\n")
	configBody := strings.Join([]string{
		"schema_version: v0.1",
		"tracker:",
		"  adapter: beads",
		"  profile: compact",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	restore := newBeadsSyncAdapter
	adapter := &captureBeadsAdapter{}
	newBeadsSyncAdapter = func() beadsSyncAdapter { return adapter }
	defer func() { newBeadsSyncAdapter = restore }()

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync", "beads", "--plan", planPath, "--state-dir", stateDir, "--profile", "agentic"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}
	if adapter.profile != tracker.RenderProfileAgentic {
		t.Fatalf("expected CLI profile override to set agentic profile, got %q", adapter.profile)
	}
}

func TestResolveSyncSelectionPrefersAdapterFlagOverConfig(t *testing.T) {
	cfg := config.Resolved{
		Tracker: config.TrackerResolved{
			Adapter: "linear",
			Profile: "compact",
		},
	}

	adapter, profile, err := resolveSyncSelection("", "beads", "", cfg)
	if err != nil {
		t.Fatalf("resolve sync selection: %v", err)
	}
	if adapter != "beads" {
		t.Fatalf("expected explicit adapter flag to win, got %q", adapter)
	}
	if profile != "compact" {
		t.Fatalf("expected config profile fallback when no flag provided, got %q", profile)
	}
}

func TestResolveSyncSelectionRejectsConflictingTargetAndAdapterFlag(t *testing.T) {
	_, _, err := resolveSyncSelection("beads", "linear", "", config.Resolved{})
	if err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("expected target/adapter conflict error, got %v", err)
	}
}

func TestResolveSyncSelectionSupportsLinear(t *testing.T) {
	adapter, profile, err := resolveSyncSelection("linear", "", "agentic", config.Resolved{})
	if err != nil {
		t.Fatalf("resolve sync selection: %v", err)
	}
	if adapter != "linear" {
		t.Fatalf("expected linear adapter, got %q", adapter)
	}
	if profile != "agentic" {
		t.Fatalf("expected explicit profile to win, got %q", profile)
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
	installCaptureBeadsAdapter(t)
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

func TestBeadProvenanceOnlyChangeIsNoop(t *testing.T) {
	installCaptureBeadsAdapter(t)
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
			NoopCount int `json:"noop_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode dry-run json: %v output=%q", err, out.String())
	}
	if payload.Data.NoopCount != 1 {
		t.Fatalf("expected noop_count=1, got %d output=%q", payload.Data.NoopCount, out.String())
	}
	if len(payload.Data.PlannedOps) != 1 {
		t.Fatalf("expected one planned op, got %d output=%q", len(payload.Data.PlannedOps), out.String())
	}
	op := payload.Data.PlannedOps[0]
	if op.Kind != "no-op" || op.ID != "fixture.task.provenance" {
		t.Fatalf("expected no-op for fixture.task.provenance, got %+v", op)
	}
	if op.Reason != "projection unchanged" {
		t.Fatalf("expected projection unchanged reason, got %q", op.Reason)
	}
}

func TestSyncBeadsAcceptsTargetBeforeFlags(t *testing.T) {
	installCaptureBeadsAdapter(t)
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
	installCaptureBeadsAdapter(t)
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
	installCaptureBeadsAdapter(t)
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
	installCaptureBeadsAdapter(t)
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

func TestSyncBeadsRestoresClosedCurrentTaskOnNoopApply(t *testing.T) {
	adapter := installCaptureBeadsAdapter(t)
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")
	planBody := strings.Join([]string{
		"- [ ] Task sync restore",
		"  @id fixture.task.sync.restore",
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

	adapter.closedIDs["fixture.task.sync.restore"] = true
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
	if payload.Data.NoopCount != 1 {
		t.Fatalf("expected noop_count=1 on unchanged semantic task, got %#v", payload.Data)
	}
	if payload.Data.TasksMutated != 1 {
		t.Fatalf("expected noop apply to restore closed current task, got %#v", payload.Data)
	}
	if adapter.closedIDs["fixture.task.sync.restore"] {
		t.Fatalf("expected sync restore to clear closed tracker state")
	}
}

func TestSyncBeadsClosesCanonicallyDoneTaskAndReopensWhenReturnedToOpen(t *testing.T) {
	adapter := installCaptureBeadsAdapter(t)
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")
	donePlan := strings.Join([]string{
		"## Completed task",
		"@id fixture.task.sync.done",
		"@status done",
		"@horizon now",
		"@accept cmd:go test ./...",
	}, "\n")
	openPlan := strings.Join([]string{
		"## Completed task",
		"@id fixture.task.sync.done",
		"@status open",
		"@horizon now",
		"@accept cmd:go test ./...",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(donePlan), 0o644); err != nil {
		t.Fatalf("write done plan: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync", "beads", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected done sync exit 0, got %d stderr=%q", exit, errOut.String())
	}
	if !adapter.closedIDs["fixture.task.sync.done"] {
		t.Fatalf("expected canonically done task to be closed in tracker state")
	}

	if err := os.WriteFile(planPath, []byte(openPlan), 0o644); err != nil {
		t.Fatalf("write open plan: %v", err)
	}
	out.Reset()
	errOut.Reset()
	exit = Run([]string{"sync", "beads", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected reopened sync exit 0, got %d stderr=%q", exit, errOut.String())
	}
	if adapter.closedIDs["fixture.task.sync.done"] {
		t.Fatalf("expected returning canonical status to open to reopen tracker state")
	}
}

func TestSyncBeadsTaskDetailsChangeCountsAsUpdate(t *testing.T) {
	adapter := installCaptureBeadsAdapter(t)
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")
	initialPlan := strings.Join([]string{
		"## Context root",
		"@id pm.context.root",
		"@horizon now",
		"",
		"Initial detail paragraph.",
	}, "\n")
	updatedPlan := strings.Join([]string{
		"## Context root",
		"@id pm.context.root",
		"@horizon now",
		"",
		"Updated detail paragraph with new semantic meaning.",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(initialPlan), 0o644); err != nil {
		t.Fatalf("write initial plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync", "beads", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected initial run exit 0, got %d stderr=%q", exit, errOut.String())
	}
	if len(adapter.pushed) != 1 {
		t.Fatalf("expected one initial push, got %#v", adapter.pushed)
	}
	firstBody := append([]string(nil), adapter.pushed[0].Sections[0].Body...)

	if err := os.WriteFile(planPath, []byte(updatedPlan), 0o644); err != nil {
		t.Fatalf("write updated plan fixture: %v", err)
	}
	out.Reset()
	errOut.Reset()
	exit = Run([]string{"sync", "beads", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected updated run exit 0, got %d stderr=%q", exit, errOut.String())
	}
	if len(adapter.pushed) != 2 {
		t.Fatalf("expected second sync to push updated task projection, got %#v", adapter.pushed)
	}
	secondBody := adapter.pushed[1].Sections[0].Body
	if reflect.DeepEqual(firstBody, secondBody) {
		t.Fatalf("expected semantic task body change to alter projected section body\nfirst=%#v\nsecond=%#v", firstBody, secondBody)
	}
	var payload struct {
		Data struct {
			UpdateCount int `json:"update_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode updated run output: %v output=%q", err, out.String())
	}
	if payload.Data.UpdateCount != 1 {
		t.Fatalf("expected detail change to be classified as update, got %#v", payload.Data)
	}
}

func TestSyncBeadsMarkStaleClosesTrackerIssueAndDropsManifestEntry(t *testing.T) {
	adapter := installCaptureBeadsAdapter(t)
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")
	if err := os.WriteFile(planPath, []byte("# Empty\n"), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	manifestPath := filepath.Join(stateDir, "sync", "beads-manifest.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	manifest := tracker.SyncManifest{
		SchemaVersion: tracker.SyncManifestSchemaVersionV01,
		Entries: []tracker.SyncManifestEntry{
			{
				ID:              "fixture.task.sync.stale",
				RemoteID:        "bd-123",
				ProjectionHash:  "old-hash",
				NodeRef:         "./PLAN.md|heading|old#1",
				SourcePath:      "./PLAN.md",
				SourceStartLine: 3,
				SourceEndLine:   9,
				SourceHash:      strings.Repeat("a", 64),
				CompileID:       strings.Repeat("b", 64),
			},
		},
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(manifestPath, raw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync", "beads", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}

	if !reflect.DeepEqual(adapter.staled, []string{"fixture.task.sync.stale"}) {
		t.Fatalf("expected stale handler to close tracked issue, got %#v", adapter.staled)
	}
	if got := adapter.staleReasons["fixture.task.sync.stale"]; !strings.Contains(got, "missing in desired") {
		t.Fatalf("expected stale reason to mention missing desired state, got %q", got)
	}

	var payload struct {
		Data struct {
			TasksMutated   int `json:"tasks_mutated"`
			MarkStaleCount int `json:"mark_stale_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json output: %v output=%q", err, out.String())
	}
	if payload.Data.MarkStaleCount != 1 {
		t.Fatalf("expected one mark-stale op, got %#v", payload.Data)
	}
	if payload.Data.TasksMutated != 1 {
		t.Fatalf("expected stale close to count as mutation, got %#v", payload.Data)
	}

	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var rewritten tracker.SyncManifest
	if err := json.Unmarshal(manifestRaw, &rewritten); err != nil {
		t.Fatalf("decode rewritten manifest: %v", err)
	}
	if len(rewritten.Entries) != 0 {
		t.Fatalf("expected stale entry to be removed from manifest, got %s", string(manifestRaw))
	}
}

func TestSyncBeadsMarkStaleClosesTrackerIssueMissingFromManifest(t *testing.T) {
	adapter := installCaptureBeadsAdapter(t)
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")
	if err := os.WriteFile(planPath, []byte("# Empty\n"), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}
	adapter.staleCandidates = []tracker.SyncManifestEntry{
		{
			ID:       "fixture.task.sync.orphan",
			RemoteID: "bd-orphan",
		},
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync", "beads", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}

	if !reflect.DeepEqual(adapter.staled, []string{"fixture.task.sync.orphan"}) {
		t.Fatalf("expected orphan tracker issue to be marked stale, got %#v", adapter.staled)
	}
	if got := adapter.staleReasons["fixture.task.sync.orphan"]; !strings.Contains(got, "missing in desired") {
		t.Fatalf("expected stale reason to mention missing desired state, got %q", got)
	}

	manifestPath := filepath.Join(stateDir, "sync", "beads-manifest.json")
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var rewritten tracker.SyncManifest
	if err := json.Unmarshal(manifestRaw, &rewritten); err != nil {
		t.Fatalf("decode rewritten manifest: %v", err)
	}
	if len(rewritten.Entries) != 0 {
		t.Fatalf("expected orphan stale candidate to stay out of manifest, got %s", string(manifestRaw))
	}
}

func TestSyncBeadsReconcilesStaleManifestEntriesBeforePlanning(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")
	planBody := strings.Join([]string{
		"- [ ] Task sync recreate",
		"  @id fixture.task.sync.recreate",
		"  @horizon now",
		"  @accept cmd:go test ./...",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}
	manifestPath := filepath.Join(stateDir, "sync", "beads-manifest.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	manifest := tracker.SyncManifest{
		SchemaVersion: tracker.SyncManifestSchemaVersionV01,
		Entries: []tracker.SyncManifestEntry{
			{
				ID:             "fixture.task.sync.recreate",
				RemoteID:       "beads:fixture.task.sync.recreate",
				ProjectionHash: "same-hash-would-have-caused-noop",
			},
		},
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(manifestPath, raw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	restore := newBeadsSyncAdapter
	adapter := &captureBeadsAdapter{
		reconcileFn: func(manifest tracker.SyncManifest) tracker.SyncManifest {
			return tracker.SyncManifest{SchemaVersion: manifest.SchemaVersion}
		},
	}
	newBeadsSyncAdapter = func() beadsSyncAdapter { return adapter }
	defer func() { newBeadsSyncAdapter = restore }()

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync", "beads", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}
	if len(adapter.pushed) != 1 {
		t.Fatalf("expected stale manifest reconciliation to force one push, got %#v", adapter.pushed)
	}

	var payload struct {
		Data struct {
			CreateCount  int `json:"create_count"`
			UpdateCount  int `json:"update_count"`
			NoopCount    int `json:"noop_count"`
			TasksMutated int `json:"tasks_mutated"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json output: %v output=%q", err, out.String())
	}
	if payload.Data.CreateCount != 1 || payload.Data.TasksMutated != 1 {
		t.Fatalf("expected recreated task to count as a create mutation, got %s", out.String())
	}
	if payload.Data.UpdateCount != 0 || payload.Data.NoopCount != 0 {
		t.Fatalf("expected no stale no-op/update accounting, got %s", out.String())
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
	adapter := installCaptureBeadsAdapter(t)
	adapter.SetPushFailures("fixture.task.sync.retry.transient", []error{
		fmt.Errorf("%w: temporary gateway timeout", tracker.ErrTransientSync),
	})
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

func TestSyncBeadsRecoversAfterManifestLossAndPlanDeletion(t *testing.T) {
	adapter := installCaptureBeadsAdapter(t)
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")
	initialPlan := strings.Join([]string{
		"## Current task",
		"@id fixture.task.sync.keep",
		"@horizon now",
		"",
		"Current task body.",
		"",
		"## Removed task",
		"@id fixture.task.sync.remove",
		"@horizon now",
		"",
		"Removed task body.",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(initialPlan), 0o644); err != nil {
		t.Fatalf("write initial plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync", "beads", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected initial sync exit 0, got %d stderr=%q", exit, errOut.String())
	}
	if len(adapter.pushed) != 2 {
		t.Fatalf("expected two initial pushes, got %#v", adapter.pushed)
	}

	updatedPlan := strings.Join([]string{
		"## Current task",
		"@id fixture.task.sync.keep",
		"@horizon now",
		"",
		"Current task body.",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(updatedPlan), 0o644); err != nil {
		t.Fatalf("write updated plan fixture: %v", err)
	}

	manifestPath := filepath.Join(stateDir, "sync", "beads-manifest.json")
	if err := os.Remove(manifestPath); err != nil {
		t.Fatalf("remove manifest fixture: %v", err)
	}
	adapter.staleCandidates = []tracker.SyncManifestEntry{
		{
			ID:       "fixture.task.sync.remove",
			RemoteID: "bd-remove",
		},
	}

	out.Reset()
	errOut.Reset()
	exit = Run([]string{"sync", "beads", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected recovery sync exit 0, got %d stderr=%q", exit, errOut.String())
	}
	if !reflect.DeepEqual(adapter.staled, []string{"fixture.task.sync.remove"}) {
		t.Fatalf("expected deleted task to be retired from tracker after manifest loss, got %#v", adapter.staled)
	}
	if len(adapter.pushed) != 3 {
		t.Fatalf("expected current task to be recreated after manifest loss, got %#v", adapter.pushed)
	}
	if adapter.pushed[2].ID != "fixture.task.sync.keep" {
		t.Fatalf("expected recreated current task push after manifest loss, got %#v", adapter.pushed[2])
	}

	var payload struct {
		Data struct {
			CreateCount    int `json:"create_count"`
			UpdateCount    int `json:"update_count"`
			NoopCount      int `json:"noop_count"`
			MarkStaleCount int `json:"mark_stale_count"`
			TasksMutated   int `json:"tasks_mutated"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode recovery sync output: %v output=%q", err, out.String())
	}
	if payload.Data.CreateCount != 1 || payload.Data.MarkStaleCount != 1 {
		t.Fatalf("expected create+mark-stale recovery after manifest loss, got %#v", payload.Data)
	}
	if payload.Data.UpdateCount != 0 || payload.Data.NoopCount != 0 {
		t.Fatalf("expected no update/no-op during manifest-loss recovery, got %#v", payload.Data)
	}
	if payload.Data.TasksMutated != 1 {
		t.Fatalf("expected stale retirement to count as the only necessary mutation after remote self-healing, got %#v", payload.Data)
	}

	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read rewritten manifest: %v", err)
	}
	var rewritten tracker.SyncManifest
	if err := json.Unmarshal(manifestRaw, &rewritten); err != nil {
		t.Fatalf("decode rewritten manifest: %v", err)
	}
	if len(rewritten.Entries) != 1 || rewritten.Entries[0].ID != "fixture.task.sync.keep" {
		t.Fatalf("expected manifest to retain only current task after recovery, got %s", string(manifestRaw))
	}
}

func TestSyncBeadsDuplicateDesiredIDsSurfaceConflictWithoutMutation(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")
	planBody := strings.Join([]string{
		"## First duplicate",
		"@id fixture.task.sync.duplicate",
		"@horizon now",
		"",
		"First body.",
		"",
		"## Second duplicate",
		"@id fixture.task.sync.duplicate",
		"@horizon now",
		"",
		"Second body.",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync", "beads", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
	if exit != protocol.ExitValidationFailed {
		t.Fatalf("expected duplicate-id sync to fail validation, got %d stderr=%q stdout=%q", exit, errOut.String(), out.String())
	}
	if out.Len() != 0 {
		t.Fatalf("expected no sync payload on duplicate desired id validation failure, got %q", out.String())
	}
	if !strings.Contains(errOut.String(), string(diag.CodeDuplicateTaskID)) {
		t.Fatalf("expected duplicate task diagnostic, got %q", errOut.String())
	}

	manifestPath := filepath.Join(stateDir, "sync", "beads-manifest.json")
	if _, err := os.Stat(manifestPath); !os.IsNotExist(err) {
		t.Fatalf("expected no manifest write on duplicate desired id validation failure, stat err=%v", err)
	}
}

func TestSyncBeadsDuplicatePriorManifestEntriesSurfaceConflictWithoutMutation(t *testing.T) {
	adapter := installCaptureBeadsAdapter(t)
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")
	planBody := strings.Join([]string{
		"## Prior duplicate task",
		"@id fixture.task.sync.prior-duplicate",
		"@horizon now",
		"",
		"Body remains unchanged.",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	manifestPath := filepath.Join(stateDir, "sync", "beads-manifest.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	manifest := tracker.SyncManifest{
		SchemaVersion: tracker.SyncManifestSchemaVersionV01,
		Entries: []tracker.SyncManifestEntry{
			{ID: "fixture.task.sync.prior-duplicate", RemoteID: "bd-1", ProjectionHash: "hash-a"},
			{ID: "fixture.task.sync.prior-duplicate", RemoteID: "bd-2", ProjectionHash: "hash-b"},
		},
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(manifestPath, raw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync", "beads", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected duplicate-prior sync to exit 0 with surfaced conflict, got %d stderr=%q", exit, errOut.String())
	}
	if len(adapter.pushed) != 0 || len(adapter.staled) != 0 {
		t.Fatalf("expected duplicate prior identity to block tracker mutation, got pushed=%#v staled=%#v", adapter.pushed, adapter.staled)
	}
	if len(adapter.dependencySynced) != 0 {
		t.Fatalf("expected duplicate prior identity to block dependency mutation, got %#v", adapter.dependencySynced)
	}

	var payload struct {
		Data struct {
			CreateCount   int `json:"create_count"`
			UpdateCount   int `json:"update_count"`
			NoopCount     int `json:"noop_count"`
			ConflictCount int `json:"conflict_count"`
			TasksMutated  int `json:"tasks_mutated"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode duplicate-prior output: %v output=%q", err, out.String())
	}
	if payload.Data.ConflictCount != 1 {
		t.Fatalf("expected one surfaced conflict for duplicate prior id, got %#v", payload.Data)
	}
	if payload.Data.CreateCount != 0 || payload.Data.UpdateCount != 0 || payload.Data.NoopCount != 0 || payload.Data.TasksMutated != 0 {
		t.Fatalf("expected duplicate prior id to avoid tracker mutation, got %#v", payload.Data)
	}
}

func TestSyncBeadsDuplicateStaleTrackerCandidatesSurfaceConflict(t *testing.T) {
	adapter := installCaptureBeadsAdapter(t)
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")
	if err := os.WriteFile(planPath, []byte("# Empty\n"), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}
	adapter.staleCandidates = []tracker.SyncManifestEntry{
		{ID: "fixture.task.sync.duplicate-stale", RemoteID: "bd-1"},
		{ID: "fixture.task.sync.duplicate-stale", RemoteID: "bd-2"},
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync", "beads", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected duplicate stale-candidate sync to exit 0 with surfaced conflict, got %d stderr=%q", exit, errOut.String())
	}
	if len(adapter.staled) != 0 || len(adapter.pushed) != 0 {
		t.Fatalf("expected duplicate stale tracker candidates to block mutation, got staled=%#v pushed=%#v", adapter.staled, adapter.pushed)
	}
	if len(adapter.dependencySynced) != 0 {
		t.Fatalf("expected duplicate stale tracker candidates to block dependency mutation, got %#v", adapter.dependencySynced)
	}

	var payload struct {
		Data struct {
			MarkStaleCount int `json:"mark_stale_count"`
			ConflictCount  int `json:"conflict_count"`
			TasksMutated   int `json:"tasks_mutated"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode duplicate stale-candidate output: %v output=%q", err, out.String())
	}
	if payload.Data.ConflictCount != 1 {
		t.Fatalf("expected one surfaced conflict for duplicate stale tracker candidates, got %#v", payload.Data)
	}
	if payload.Data.MarkStaleCount != 0 || payload.Data.TasksMutated != 0 {
		t.Fatalf("expected duplicate stale tracker candidates to avoid stale mutation, got %#v", payload.Data)
	}
}

func TestSyncBeadsManifestAndMatchingStaleCandidateDoNotConflict(t *testing.T) {
	adapter := installCaptureBeadsAdapter(t)
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")
	if err := os.WriteFile(planPath, []byte("# Empty\n"), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	manifestPath := filepath.Join(stateDir, "sync", "beads-manifest.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	manifest := tracker.SyncManifest{
		SchemaVersion: tracker.SyncManifestSchemaVersionV01,
		Entries: []tracker.SyncManifestEntry{
			{ID: "fixture.task.sync.deleted", RemoteID: "bd-1", ProjectionHash: "hash-a"},
		},
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(manifestPath, raw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	adapter.staleCandidates = []tracker.SyncManifestEntry{
		{ID: "fixture.task.sync.deleted", RemoteID: "bd-1", ProjectionHash: "hash-a"},
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync", "beads", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected matching manifest/stale candidate sync to exit 0, got %d stderr=%q", exit, errOut.String())
	}
	if !reflect.DeepEqual(adapter.staled, []string{"fixture.task.sync.deleted"}) {
		t.Fatalf("expected deleted task to be marked stale once, got %#v", adapter.staled)
	}

	var payload struct {
		Data struct {
			MarkStaleCount int `json:"mark_stale_count"`
			ConflictCount  int `json:"conflict_count"`
			TasksMutated   int `json:"tasks_mutated"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode matching manifest/stale output: %v output=%q", err, out.String())
	}
	if payload.Data.ConflictCount != 0 {
		t.Fatalf("expected no conflict for matching manifest and stale candidate, got %#v", payload.Data)
	}
	if payload.Data.MarkStaleCount != 1 || payload.Data.TasksMutated != 1 {
		t.Fatalf("expected one stale mutation for deleted task, got %#v", payload.Data)
	}
}

func TestSyncBeadsDeletesTaskByPositionAcrossNAndNPlusOnePlans(t *testing.T) {
	type taskSpec struct {
		ID    string
		Title string
	}

	buildPlan := func(tasks []taskSpec) string {
		lines := []string{"# Deletion Matrix", ""}
		for i, task := range tasks {
			lines = append(lines,
				fmt.Sprintf("## Section %d", i+1),
				fmt.Sprintf("- [ ] %s", task.Title),
				fmt.Sprintf("  @id %s", task.ID),
				"  @horizon now",
				"  @accept cmd:go test ./...",
				"",
			)
		}
		return strings.Join(lines, "\n")
	}

	cases := []struct {
		name      string
		tasks     []taskSpec
		deleteIdx int
	}{
		{name: "n=2 delete first", tasks: []taskSpec{{ID: "fixture.task.a", Title: "Task A"}, {ID: "fixture.task.b", Title: "Task B"}}, deleteIdx: 0},
		{name: "n=2 delete last", tasks: []taskSpec{{ID: "fixture.task.a", Title: "Task A"}, {ID: "fixture.task.b", Title: "Task B"}}, deleteIdx: 1},
		{name: "n+1 delete first", tasks: []taskSpec{{ID: "fixture.task.a", Title: "Task A"}, {ID: "fixture.task.b", Title: "Task B"}, {ID: "fixture.task.c", Title: "Task C"}}, deleteIdx: 0},
		{name: "n+1 delete middle", tasks: []taskSpec{{ID: "fixture.task.a", Title: "Task A"}, {ID: "fixture.task.b", Title: "Task B"}, {ID: "fixture.task.c", Title: "Task C"}}, deleteIdx: 1},
		{name: "n+1 delete last", tasks: []taskSpec{{ID: "fixture.task.a", Title: "Task A"}, {ID: "fixture.task.b", Title: "Task B"}, {ID: "fixture.task.c", Title: "Task C"}}, deleteIdx: 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			adapter := installCaptureBeadsAdapter(t)
			tmp := t.TempDir()
			planPath := filepath.Join(tmp, "PLAN.md")
			stateDir := filepath.Join(tmp, ".planmark")

			initialPlan := buildPlan(tc.tasks)
			if err := os.WriteFile(planPath, []byte(initialPlan), 0o644); err != nil {
				t.Fatalf("write initial plan: %v", err)
			}

			var out bytes.Buffer
			var errOut bytes.Buffer
			exit := Run([]string{"sync", "beads", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
			if exit != 0 {
				t.Fatalf("expected initial sync success, got %d stderr=%q", exit, errOut.String())
			}

			deletedID := tc.tasks[tc.deleteIdx].ID
			remainingTasks := append([]taskSpec(nil), tc.tasks[:tc.deleteIdx]...)
			remainingTasks = append(remainingTasks, tc.tasks[tc.deleteIdx+1:]...)
			if err := os.WriteFile(planPath, []byte(buildPlan(remainingTasks)), 0o644); err != nil {
				t.Fatalf("write updated plan: %v", err)
			}

			adapter.pushed = nil
			adapter.staled = nil
			adapter.dependencySynced = nil

			out.Reset()
			errOut.Reset()
			exit = Run([]string{"sync", "beads", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
			if exit != 0 {
				t.Fatalf("expected deletion sync success, got %d stderr=%q", exit, errOut.String())
			}

			if !reflect.DeepEqual(adapter.staled, []string{deletedID}) {
				t.Fatalf("expected exactly deleted task %q to be marked stale, got %#v", deletedID, adapter.staled)
			}

			var payload struct {
				Data struct {
					MarkStaleCount int `json:"mark_stale_count"`
					ConflictCount  int `json:"conflict_count"`
					TasksMutated   int `json:"tasks_mutated"`
				} `json:"data"`
			}
			if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
				t.Fatalf("decode deletion sync output: %v output=%q", err, out.String())
			}
			if payload.Data.ConflictCount != 0 {
				t.Fatalf("expected no conflict for deletion of %q in %v, got %#v", deletedID, tc.tasks, payload.Data)
			}
			if payload.Data.MarkStaleCount != 1 || payload.Data.TasksMutated != 1 {
				t.Fatalf("expected one stale mutation for deletion of %q in %v, got %#v", deletedID, tc.tasks, payload.Data)
			}

			manifestPath := filepath.Join(stateDir, "sync", "beads-manifest.json")
			manifestRaw, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatalf("read manifest: %v", err)
			}
			var manifest tracker.SyncManifest
			if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
				t.Fatalf("decode manifest: %v", err)
			}
			gotIDs := make([]string, 0, len(manifest.Entries))
			for _, entry := range manifest.Entries {
				gotIDs = append(gotIDs, entry.ID)
			}
			slices.Sort(gotIDs)
			wantIDs := make([]string, 0, len(remainingTasks))
			for _, task := range remainingTasks {
				wantIDs = append(wantIDs, task.ID)
			}
			slices.Sort(wantIDs)
			if !reflect.DeepEqual(gotIDs, wantIDs) {
				t.Fatalf("expected manifest IDs %#v after deleting %q, got %#v", wantIDs, deletedID, gotIDs)
			}
		})
	}
}

func TestSyncBeadsDeleteThenReAddSameIDDoesNotConflict(t *testing.T) {
	adapter := installCaptureBeadsAdapter(t)
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")

	initialPlan := strings.Join([]string{
		"# ReAdd Matrix",
		"",
		"## First",
		"- [ ] Task A",
		"  @id fixture.task.a",
		"  @horizon now",
		"  @accept cmd:go test ./...",
		"",
		"## Second",
		"- [ ] Task B",
		"  @id fixture.task.b",
		"  @horizon now",
		"  @accept cmd:go test ./...",
	}, "\n")
	deletedPlan := strings.Join([]string{
		"# ReAdd Matrix",
		"",
		"## Second",
		"- [ ] Task B",
		"  @id fixture.task.b",
		"  @horizon now",
		"  @accept cmd:go test ./...",
	}, "\n")

	if err := os.WriteFile(planPath, []byte(initialPlan), 0o644); err != nil {
		t.Fatalf("write initial plan: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync", "beads", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected initial sync success, got %d stderr=%q", exit, errOut.String())
	}

	if err := os.WriteFile(planPath, []byte(deletedPlan), 0o644); err != nil {
		t.Fatalf("write deleted plan: %v", err)
	}
	adapter.pushed = nil
	adapter.staled = nil
	out.Reset()
	errOut.Reset()
	exit = Run([]string{"sync", "beads", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected deletion sync success, got %d stderr=%q", exit, errOut.String())
	}
	if !reflect.DeepEqual(adapter.staled, []string{"fixture.task.a"}) {
		t.Fatalf("expected fixture.task.a to be retired on deletion, got %#v", adapter.staled)
	}

	if err := os.WriteFile(planPath, []byte(initialPlan), 0o644); err != nil {
		t.Fatalf("write restored plan: %v", err)
	}
	adapter.pushed = nil
	adapter.staled = nil
	out.Reset()
	errOut.Reset()
	exit = Run([]string{"sync", "beads", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected re-add sync success, got %d stderr=%q", exit, errOut.String())
	}

	var payload struct {
		Data struct {
			CreateCount   int `json:"create_count"`
			UpdateCount   int `json:"update_count"`
			NoopCount     int `json:"noop_count"`
			MarkStaleCount int `json:"mark_stale_count"`
			ConflictCount int `json:"conflict_count"`
			TasksMutated  int `json:"tasks_mutated"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode re-add sync output: %v output=%q", err, out.String())
	}
	if payload.Data.ConflictCount != 0 {
		t.Fatalf("expected no conflict when re-adding same id, got %#v", payload.Data)
	}
	if payload.Data.CreateCount != 1 || payload.Data.MarkStaleCount != 0 {
		t.Fatalf("expected re-add to recreate exactly one task, got %#v", payload.Data)
	}
	if payload.Data.TasksMutated != 1 {
		t.Fatalf("expected only re-added task to mutate, got %#v", payload.Data)
	}
	if len(adapter.staled) != 0 {
		t.Fatalf("expected no stale mutation during re-add, got %#v", adapter.staled)
	}
	if len(adapter.pushed) != 2 {
		t.Fatalf("expected sync to consider both desired tasks on re-add, got %#v", adapter.pushed)
	}

	manifestPath := filepath.Join(stateDir, "sync", "beads-manifest.json")
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest tracker.SyncManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	gotIDs := make([]string, 0, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		gotIDs = append(gotIDs, entry.ID)
	}
	slices.Sort(gotIDs)
	if !reflect.DeepEqual(gotIDs, []string{"fixture.task.a", "fixture.task.b"}) {
		t.Fatalf("expected both IDs in manifest after re-add, got %#v", gotIDs)
	}
}

func TestSyncBeadsWarnsOnUnknownDependencyAndContinues(t *testing.T) {
	adapter := installCaptureBeadsAdapter(t)
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")
	planBody := strings.Join([]string{
		"# Demo",
		"",
		"## Publish docs",
		"- [ ] Add public launch walkthrough",
		"  @id demo.docs.launch",
		"  @horizon later",
		"  @deps demo.sync.beads",
		"  @accept cmd:test -f README.md",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"sync", "beads", "--plan", planPath, "--state-dir", stateDir, "--format", "json"}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected sync success with dependency warning, got %d stderr=%q stdout=%q", exit, errOut.String(), out.String())
	}
	if !strings.Contains(errOut.String(), "warning "+string(diag.CodeUnknownDependency)) {
		t.Fatalf("expected UNKNOWN_DEPENDENCY warning, got %q", errOut.String())
	}
	if len(adapter.pushed) != 1 {
		t.Fatalf("expected dependent task to remain syncable, got %#v", adapter.pushed)
	}

	var payload struct {
		Data struct {
			CreateCount   int `json:"create_count"`
			ConflictCount int `json:"conflict_count"`
			TasksMutated  int `json:"tasks_mutated"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode warning sync output: %v output=%q", err, out.String())
	}
	if payload.Data.ConflictCount != 0 {
		t.Fatalf("expected no conflict for unknown dependency warning path, got %#v", payload.Data)
	}
	if payload.Data.CreateCount != 1 || payload.Data.TasksMutated != 1 {
		t.Fatalf("expected dependent task to still sync as create, got %#v", payload.Data)
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
	adapter := installCaptureBeadsAdapter(t)
	adapter.SetPushFailures("fixture.task.sync.retry.ratelimit", []error{
		fmt.Errorf("%w: 429", tracker.ErrRateLimitedSync),
		fmt.Errorf("%w: 429", tracker.ErrRateLimitedSync),
	})
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
