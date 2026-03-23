package tracker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/cache"
)

func TestBeadsProjectionPayloadContainsHashes(t *testing.T) {
	task := TaskProjection{
		ID:      "fixture.task.now",
		Title:   "Implement deterministic output",
		Horizon: "now",
		Anchor:  "testdata/plans/mixed.md#L12",
		Provenance: TaskProvenance{
			Path:       "testdata/plans/mixed.md",
			StartLine:  12,
			EndLine:    14,
			SourceHash: strings.Repeat("a", 64),
		},
		Dependencies: []string{"dep.schema", "dep.runtime"},
		Acceptance: []string{
			"cmd:go test ./... -run TestCompile",
			"file:.planmark/tmp/plan.json exists",
		},
		Steps: []TaskProjectionStep{
			{NodeRef: "node.step.1", Title: "Write migration"},
			{NodeRef: "node.step.2", Title: "Verify rollback", Checked: true},
		},
		Evidence: []TaskProjectionEvidence{
			{NodeRef: "node.evidence.1"},
			{NodeRef: "node.evidence.2"},
		},
	}

	payload, err := BuildProjectionPayload(task)
	if err != nil {
		t.Fatalf("build projection payload: %v", err)
	}

	if payload.ProjectionSchemaVersion != ProjectionSchemaVersionV02 {
		t.Fatalf("expected projection schema version %q, got %q", ProjectionSchemaVersionV02, payload.ProjectionSchemaVersion)
	}
	if payload.ID != task.ID {
		t.Fatalf("expected id %q, got %q", task.ID, payload.ID)
	}
	if payload.Anchor == "" {
		t.Fatalf("expected non-empty anchor")
	}
	if payload.SourceRange.Path != task.Provenance.Path {
		t.Fatalf("expected source path %q, got %q", task.Provenance.Path, payload.SourceRange.Path)
	}
	if payload.SourceRange.StartLine != task.Provenance.StartLine || payload.SourceRange.EndLine != task.Provenance.EndLine {
		t.Fatalf("expected source range %d-%d, got %d-%d", task.Provenance.StartLine, task.Provenance.EndLine, payload.SourceRange.StartLine, payload.SourceRange.EndLine)
	}
	if payload.SourceHash != task.Provenance.SourceHash {
		t.Fatalf("expected source hash %q, got %q", task.Provenance.SourceHash, payload.SourceHash)
	}
	if payload.Horizon != task.Horizon {
		t.Fatalf("expected horizon %q, got %q", task.Horizon, payload.Horizon)
	}
	if !reflect.DeepEqual(payload.Dependencies, task.Dependencies) {
		t.Fatalf("expected dependencies %#v, got %#v", task.Dependencies, payload.Dependencies)
	}
	if payload.AcceptanceDigest == "" {
		t.Fatalf("expected non-empty acceptance digest")
	}
	if len(payload.AcceptanceDigest) != 64 {
		t.Fatalf("expected sha256 hex digest length 64, got %d", len(payload.AcceptanceDigest))
	}
	if len(payload.Steps) != 2 {
		t.Fatalf("expected two projected steps, got %#v", payload.Steps)
	}
	if payload.Steps[1].Title != "Verify rollback" || !payload.Steps[1].Checked {
		t.Fatalf("expected ordered step projection, got %#v", payload.Steps)
	}
	if !reflect.DeepEqual(payload.EvidenceNodeRefs, []string{"node.evidence.1", "node.evidence.2"}) {
		t.Fatalf("expected evidence refs %#v, got %#v", []string{"node.evidence.1", "node.evidence.2"}, payload.EvidenceNodeRefs)
	}
}

func TestBeadsProjectionPayloadRespectsProjectionVersion(t *testing.T) {
	task := TaskProjection{
		ID:    "fixture.task.version",
		Title: "Projection version passthrough",
		Provenance: TaskProvenance{
			Path:       "testdata/plans/mixed.md",
			StartLine:  3,
			EndLine:    4,
			SourceHash: strings.Repeat("c", 64),
		},
		ProjectionVersion: "v0.2",
	}

	payload, err := BuildProjectionPayload(task)
	if err != nil {
		t.Fatalf("build projection payload: %v", err)
	}
	if payload.ProjectionSchemaVersion != "v0.2" {
		t.Fatalf("expected projection schema version v0.2, got %q", payload.ProjectionSchemaVersion)
	}
}

func TestBeadsProjectionPayloadPreservesOrderedRicherFields(t *testing.T) {
	first := TaskProjection{
		ID:    "fixture.task.ordered",
		Title: "Ordered payload",
		Provenance: TaskProvenance{
			Path:       "testdata/plans/mixed.md",
			StartLine:  8,
			EndLine:    12,
			SourceHash: strings.Repeat("d", 64),
		},
		Dependencies: []string{"dep.a", "dep.b"},
		Steps: []TaskProjectionStep{
			{NodeRef: "node.step.1", Title: "first"},
			{NodeRef: "node.step.2", Title: "second"},
		},
		Evidence: []TaskProjectionEvidence{
			{NodeRef: "node.evidence.1"},
			{NodeRef: "node.evidence.2"},
		},
	}
	second := first
	second.Steps = []TaskProjectionStep{
		{NodeRef: "node.step.2", Title: "second"},
		{NodeRef: "node.step.1", Title: "first"},
	}
	second.Evidence = []TaskProjectionEvidence{
		{NodeRef: "node.evidence.2"},
		{NodeRef: "node.evidence.1"},
	}

	firstPayload, err := BuildProjectionPayload(first)
	if err != nil {
		t.Fatalf("build first payload: %v", err)
	}
	secondPayload, err := BuildProjectionPayload(second)
	if err != nil {
		t.Fatalf("build second payload: %v", err)
	}
	firstHash, err := projectionHash(firstPayload)
	if err != nil {
		t.Fatalf("hash first payload: %v", err)
	}
	secondHash, err := projectionHash(secondPayload)
	if err != nil {
		t.Fatalf("hash second payload: %v", err)
	}
	if firstHash == secondHash {
		t.Fatalf("expected ordered richer fields to change projection hash")
	}
}

func TestBeadsPushIdempotentOnProjectionHash(t *testing.T) {
	adapter := NewBeadsAdapter()
	ctx := context.Background()

	task := TaskProjection{
		ID:    "fixture.task.idempotent",
		Title: "Idempotent push check",
		Provenance: TaskProvenance{
			Path:       "testdata/plans/mixed.md",
			StartLine:  7,
			EndLine:    9,
			SourceHash: strings.Repeat("b", 64),
		},
		Acceptance: []string{"cmd:go test ./... -run TestCompile"},
	}

	first, err := adapter.PushTask(ctx, task)
	if err != nil {
		t.Fatalf("first push: %v", err)
	}
	if !first.Mutated || first.Noop {
		t.Fatalf("expected first push to mutate; got mutated=%v noop=%v", first.Mutated, first.Noop)
	}

	second, err := adapter.PushTask(ctx, task)
	if err != nil {
		t.Fatalf("second push: %v", err)
	}
	if second.Mutated || !second.Noop {
		t.Fatalf("expected second push to be noop; got mutated=%v noop=%v", second.Mutated, second.Noop)
	}
	if first.RemoteID == "" || second.RemoteID == "" {
		t.Fatalf("expected non-empty remote ids")
	}
	if first.RemoteID != second.RemoteID {
		t.Fatalf("expected stable remote id across idempotent pushes; got %q then %q", first.RemoteID, second.RemoteID)
	}
}

func TestBeadsPullOnlySafeFields(t *testing.T) {
	adapter := NewBeadsAdapter()
	adapter.SetRemoteRuntimeFields("fixture.task.safe", RuntimeFields{
		Status:   "in_progress",
		Assignee: "agent.orange",
		Priority: "P1",
	})

	got, err := adapter.PullRuntimeFields(context.Background(), []string{"fixture.task.safe"})
	if err != nil {
		t.Fatalf("pull runtime fields: %v", err)
	}
	state, ok := got["fixture.task.safe"]
	if !ok {
		t.Fatalf("expected runtime fields for fixture.task.safe")
	}
	if state.Status != "in_progress" || state.Assignee != "agent.orange" || state.Priority != "P1" {
		t.Fatalf("unexpected runtime fields: %#v", state)
	}
}

func TestBeadsPullUsesLastSeenRuntimeHash(t *testing.T) {
	adapter := NewBeadsAdapter()
	id := "fixture.task.runtime"

	adapter.SetRemoteRuntimeFields(id, RuntimeFields{
		Status:   "todo",
		Assignee: "agent.orange",
		Priority: "P2",
	})
	first, err := adapter.PullRuntimeFields(context.Background(), []string{id})
	if err != nil {
		t.Fatalf("first pull runtime fields: %v", err)
	}
	if _, ok := first[id]; !ok {
		t.Fatalf("expected first pull to return runtime update")
	}

	second, err := adapter.PullRuntimeFields(context.Background(), []string{id})
	if err != nil {
		t.Fatalf("second pull runtime fields: %v", err)
	}
	if _, ok := second[id]; ok {
		t.Fatalf("expected second pull to be no-op for unchanged runtime hash")
	}

	adapter.SetRemoteRuntimeFields(id, RuntimeFields{
		Status:   "done",
		Assignee: "agent.orange",
		Priority: "P2",
	})
	third, err := adapter.PullRuntimeFields(context.Background(), []string{id})
	if err != nil {
		t.Fatalf("third pull runtime fields: %v", err)
	}
	updated, ok := third[id]
	if !ok {
		t.Fatalf("expected third pull to return updated runtime fields")
	}
	if updated.Status != "done" {
		t.Fatalf("expected updated status done, got %#v", updated)
	}
}

func TestBeadsDetectProjectionDriftBySourceHashMismatch(t *testing.T) {
	adapter := NewBeadsAdapter()
	ctx := context.Background()
	task := TaskProjection{
		ID:    "fixture.task.drift",
		Title: "Detect source hash drift",
		Provenance: TaskProvenance{
			Path:       "testdata/plans/mixed.md",
			StartLine:  5,
			EndLine:    6,
			SourceHash: strings.Repeat("d", 64),
		},
		Acceptance: []string{"cmd:go test ./..."},
	}

	drifted, err := adapter.DetectProjectionDrift(task)
	if err != nil {
		t.Fatalf("detect projection drift before first push: %v", err)
	}
	if drifted {
		t.Fatalf("expected no drift before first push")
	}

	if _, err := adapter.PushTask(ctx, task); err != nil {
		t.Fatalf("first push: %v", err)
	}
	drifted, err = adapter.DetectProjectionDrift(task)
	if err != nil {
		t.Fatalf("detect projection drift on unchanged task: %v", err)
	}
	if drifted {
		t.Fatalf("expected no drift for unchanged source hash")
	}

	task.Provenance.SourceHash = strings.Repeat("e", 64)
	drifted, err = adapter.DetectProjectionDrift(task)
	if err != nil {
		t.Fatalf("detect projection drift after source change: %v", err)
	}
	if !drifted {
		t.Fatalf("expected drift when source hash changes")
	}
}

func TestBeadsPushSurfacesDriftDiagnostic(t *testing.T) {
	adapter := NewBeadsAdapter()
	ctx := context.Background()
	task := TaskProjection{
		ID:    "fixture.task.push_drift",
		Title: "Push drift diagnostic",
		Provenance: TaskProvenance{
			Path:       "testdata/plans/mixed.md",
			StartLine:  10,
			EndLine:    12,
			SourceHash: strings.Repeat("f", 64),
		},
		Acceptance: []string{"cmd:go test ./..."},
	}

	first, err := adapter.PushTask(ctx, task)
	if err != nil {
		t.Fatalf("first push: %v", err)
	}
	if first.Diagnostic != "projection updated" {
		t.Fatalf("expected normal update diagnostic, got %q", first.Diagnostic)
	}

	task.Provenance.SourceHash = strings.Repeat("0", 64)
	second, err := adapter.PushTask(ctx, task)
	if err != nil {
		t.Fatalf("second push: %v", err)
	}
	if !strings.Contains(second.Diagnostic, "drift detected") {
		t.Fatalf("expected drift diagnostic, got %q", second.Diagnostic)
	}
}

func TestBeadsWriteSyncManifest(t *testing.T) {
	adapter := NewBeadsAdapter()
	ctx := context.Background()
	firstTask := TaskProjection{
		ID:    "fixture.task.manifest.a",
		Title: "Manifest A",
		Provenance: TaskProvenance{
			NodeRef:    "testdata/plans/mixed.md|checkbox|a#1",
			Path:       "testdata/plans/mixed.md",
			StartLine:  1,
			EndLine:    2,
			SourceHash: strings.Repeat("1", 64),
			CompileID:  strings.Repeat("a", 64),
		},
	}
	secondTask := TaskProjection{
		ID:    "fixture.task.manifest.b",
		Title: "Manifest B",
		Provenance: TaskProvenance{
			NodeRef:    "testdata/plans/mixed.md|checkbox|b#1",
			Path:       "testdata/plans/mixed.md",
			StartLine:  3,
			EndLine:    4,
			SourceHash: strings.Repeat("2", 64),
			CompileID:  strings.Repeat("b", 64),
		},
	}
	if _, err := adapter.PushTask(ctx, secondTask); err != nil {
		t.Fatalf("push second task: %v", err)
	}
	if _, err := adapter.PushTask(ctx, firstTask); err != nil {
		t.Fatalf("push first task: %v", err)
	}
	adapter.SetRemoteRuntimeFields(firstTask.ID, RuntimeFields{
		Status:   "in_progress",
		Assignee: "agent.orange",
		Priority: "P1",
	})
	if _, err := adapter.PullRuntimeFields(ctx, []string{firstTask.ID}); err != nil {
		t.Fatalf("pull runtime fields: %v", err)
	}

	stateDir := filepath.Join(t.TempDir(), ".planmark")
	manifestPath, err := adapter.WriteSyncManifest(stateDir)
	if err != nil {
		t.Fatalf("write sync manifest: %v", err)
	}
	expectedPath := filepath.Join(stateDir, "sync", "beads-manifest.json")
	if manifestPath != expectedPath {
		t.Fatalf("expected manifest path %q, got %q", expectedPath, manifestPath)
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest BeadsSyncManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if manifest.SchemaVersion != BeadsManifestSchemaVersionV01 {
		t.Fatalf("expected schema version %q, got %q", BeadsManifestSchemaVersionV01, manifest.SchemaVersion)
	}
	if len(manifest.Entries) != 2 {
		t.Fatalf("expected two manifest entries, got %d", len(manifest.Entries))
	}
	if manifest.Entries[0].ID != firstTask.ID || manifest.Entries[1].ID != secondTask.ID {
		t.Fatalf("expected entries sorted by id, got %#v", manifest.Entries)
	}
	if manifest.Entries[0].LastSeenRuntimeHash == "" {
		t.Fatalf("expected first entry to include runtime hash")
	}
	if manifest.Entries[1].LastSeenRuntimeHash != "" {
		t.Fatalf("expected second entry to omit runtime hash, got %#v", manifest.Entries[1])
	}
	if manifest.Entries[0].NodeRef != firstTask.Provenance.NodeRef || manifest.Entries[1].NodeRef != secondTask.Provenance.NodeRef {
		t.Fatalf("expected node refs in manifest entries, got %#v", manifest.Entries)
	}
	if manifest.Entries[0].SourcePath != firstTask.Provenance.Path || manifest.Entries[1].SourcePath != secondTask.Provenance.Path {
		t.Fatalf("expected source paths in manifest entries, got %#v", manifest.Entries)
	}
	if manifest.Entries[0].CompileID != firstTask.Provenance.CompileID || manifest.Entries[1].CompileID != secondTask.Provenance.CompileID {
		t.Fatalf("expected compile ids in manifest entries, got %#v", manifest.Entries)
	}
}

func TestBeadsWriteSyncManifestRespectsLock(t *testing.T) {
	adapter := NewBeadsAdapter()
	stateDir := filepath.Join(t.TempDir(), ".planmark")

	lock, err := cache.AcquireLock(stateDir, "sync-beads-manifest")
	if err != nil {
		t.Fatalf("acquire preexisting lock: %v", err)
	}
	defer lock.Release()

	_, err = adapter.WriteSyncManifest(stateDir)
	if err == nil {
		t.Fatalf("expected write sync manifest to fail while lock is held")
	}
	if !errors.Is(err, cache.ErrLockHeld) {
		t.Fatalf("expected ErrLockHeld, got: %v", err)
	}
}

func TestBeadsGoldenProjection(t *testing.T) {
	task := TaskProjection{
		ID:      "fixture.task.golden_projection",
		Title:   "Golden projection payload",
		Horizon: "now",
		Provenance: TaskProvenance{
			Path:       "testdata/plans/mixed.md",
			StartLine:  21,
			EndLine:    24,
			SourceHash: strings.Repeat("9", 64),
		},
		Dependencies: []string{"dep.schema", "dep.runtime"},
		Acceptance: []string{
			"cmd:go test ./... -run TestCompile",
			"cmd:go test ./... -run TestSync",
		},
		Steps: []TaskProjectionStep{
			{NodeRef: "node.step.1", Title: "Write migration"},
			{NodeRef: "node.step.2", Title: "Verify rollback", Checked: true},
		},
		Evidence: []TaskProjectionEvidence{{NodeRef: "node.evidence.1"}},
	}
	payload, err := BuildProjectionPayload(task)
	if err != nil {
		t.Fatalf("build projection payload: %v", err)
	}
	got, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal projection payload: %v", err)
	}
	got = append(got, '\n')

	wantPath := filepath.Join("..", "..", "testdata", "tracker", "beads", "projection.request.golden.json")
	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read golden %s: %v", wantPath, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("golden mismatch for %s\nwant:\n%s\ngot:\n%s", wantPath, string(want), string(got))
	}
}

func TestBeadsGoldenPullSafeFields(t *testing.T) {
	adapter := NewBeadsAdapter()
	taskID := "fixture.task.golden_pull"
	adapter.SetRemoteRuntimeFields(taskID, RuntimeFields{
		Status:   "in_progress",
		Assignee: "agent.golden",
		Priority: "P1",
	})

	gotState, err := adapter.PullRuntimeFields(context.Background(), []string{taskID})
	if err != nil {
		t.Fatalf("pull runtime fields: %v", err)
	}
	fields, ok := gotState[taskID]
	if !ok {
		t.Fatalf("expected runtime fields for %s", taskID)
	}
	envelope := map[string]any{
		"id": taskID,
		"runtime": map[string]string{
			"status":   fields.Status,
			"assignee": fields.Assignee,
			"priority": fields.Priority,
		},
		"snapshot": fmt.Sprintf("%s|%s|%s", fields.Status, fields.Assignee, fields.Priority),
	}

	got, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		t.Fatalf("marshal runtime envelope: %v", err)
	}
	got = append(got, '\n')

	wantPath := filepath.Join("..", "..", "testdata", "tracker", "beads", "pull.response.golden.json")
	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read golden %s: %v", wantPath, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("golden mismatch for %s\nwant:\n%s\ngot:\n%s", wantPath, string(want), string(got))
	}
}
