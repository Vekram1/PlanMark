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
	"slices"
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

func TestBeadsAdapterRenderTaskProjectionUsesDefaultProfile(t *testing.T) {
	adapter := NewBeadsAdapter()
	task := TaskProjection{
		ID:      "fixture.task.rendered",
		Title:   "Rendered through beads adapter",
		Horizon: "now",
		Provenance: TaskProvenance{
			Path:       "testdata/plans/mixed.md",
			StartLine:  8,
			EndLine:    12,
			SourceHash: strings.Repeat("e", 64),
		},
		Dependencies: []string{"dep.schema"},
		Acceptance:   []string{"cmd:go test ./... -run TestCompile"},
		Sections: []TaskProjectionSection{
			{Key: "why", Body: []string{"Need a deterministic tracker body"}},
		},
		Steps: []TaskProjectionStep{
			{NodeRef: "node.step.1", Title: "Write migration"},
		},
	}

	rendered, err := adapter.RenderTaskProjection(task)
	if err != nil {
		t.Fatalf("render task projection: %v", err)
	}
	if rendered.Profile != BeadsRenderProfile {
		t.Fatalf("expected beads render profile %q, got %q", BeadsRenderProfile, rendered.Profile)
	}
	if rendered.StepMode != CapabilityRendered {
		t.Fatalf("expected beads adapter to render steps into the body, got %q", rendered.StepMode)
	}
	if len(rendered.Steps) != 0 {
		t.Fatalf("expected no native rendered steps for beads, got %#v", rendered.Steps)
	}
	body := strings.Join(rendered.Body, "\n")
	if !strings.Contains(body, "## Why") {
		t.Fatalf("expected rendered body to contain section heading, got %q", body)
	}
	if !strings.Contains(body, "## Steps") {
		t.Fatalf("expected rendered body to contain steps heading, got %q", body)
	}
	if !strings.Contains(body, "- [ ] Write migration") {
		t.Fatalf("expected rendered body to contain rendered checklist step, got %q", body)
	}
}

func TestBuildBeadsStepsFromRenderedPreservesNodeRefsForNativeSteps(t *testing.T) {
	rendered := RenderedTask{
		StepMode: CapabilityNative,
		Steps: []RenderedChecklistItem{
			{NodeRef: "node.step.1", Title: "Write migration"},
			{NodeRef: "node.step.2", Title: "Verify rollback", Checked: true},
		},
	}

	got := buildBeadsStepsFromRendered(rendered, nil)
	want := []BeadsStep{
		{NodeRef: "node.step.1", Title: "Write migration"},
		{NodeRef: "node.step.2", Title: "Verify rollback", Checked: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected native rendered steps to preserve node refs\nwant=%#v\ngot=%#v", want, got)
	}
}

func TestBeadsPushIdempotentOnProjectionHash(t *testing.T) {
	restore := runBrCommand
	defer func() { runBrCommand = restore }()
	runBrCommand = func(args ...string) ([]byte, error) {
		switch args[0] {
		case "list":
			return []byte(`[]`), nil
		case "create":
			return []byte(`{"id":"bead-101","title":"Idempotent push check"}`), nil
		default:
			t.Fatalf("unexpected br command: %#v", args)
			return nil, nil
		}
	}

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
	restore := runBrCommand
	defer func() { runBrCommand = restore }()
	runBrCommand = func(args ...string) ([]byte, error) {
		switch args[0] {
		case "list":
			return []byte(`[]`), nil
		case "create":
			return []byte(`{"id":"bead-151","title":"Detect source hash drift"}`), nil
		default:
			t.Fatalf("unexpected br command: %#v", args)
			return nil, nil
		}
	}

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
	restore := runBrCommand
	defer func() { runBrCommand = restore }()
	runBrCommand = func(args ...string) ([]byte, error) {
		switch args[0] {
		case "list":
			return []byte(`[]`), nil
		case "create":
			return []byte(`{"id":"bead-201","title":"Push drift diagnostic"}`), nil
		case "update":
			return []byte(`[{"id":"bead-201","title":"Push drift diagnostic"}]`), nil
		default:
			t.Fatalf("unexpected br command: %#v", args)
			return nil, nil
		}
	}

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

func TestBeadsReconcileSyncManifestDropsMissingRemoteEntries(t *testing.T) {
	restore := runBrCommand
	defer func() { runBrCommand = restore }()
	runBrCommand = func(args ...string) ([]byte, error) {
		if len(args) < 2 || args[0] != "show" {
			t.Fatalf("unexpected br command: %#v", args)
		}
		if args[1] == "bead-keep" {
			return []byte(`[{"id":"bead-keep","title":"Kept issue"}]`), nil
		}
		return nil, fmt.Errorf("issue not found")
	}

	adapter := NewBeadsAdapter()
	manifest := BeadsSyncManifest{
		SchemaVersion: BeadsManifestSchemaVersionV01,
		Entries: []BeadsManifestEntry{
			{ID: "fixture.task.keep", RemoteID: "bead-keep", ProjectionHash: "hash-a"},
			{ID: "fixture.task.drop", RemoteID: "bead-missing", ProjectionHash: "hash-b"},
			{ID: "fixture.task.synthetic", RemoteID: "beads:fixture.task.synthetic", ProjectionHash: "hash-c"},
		},
	}

	reconciled, err := adapter.ReconcileSyncManifest(context.Background(), manifest)
	if err != nil {
		t.Fatalf("reconcile sync manifest: %v", err)
	}
	if len(reconciled.Entries) != 1 {
		t.Fatalf("expected one surviving manifest entry, got %#v", reconciled.Entries)
	}
	if reconciled.Entries[0].ID != "fixture.task.keep" {
		t.Fatalf("expected keep entry to survive, got %#v", reconciled.Entries)
	}
}

func TestBeadsPushCreatesRealIssueAndStoresRemoteID(t *testing.T) {
	restore := runBrCommand
	defer func() { runBrCommand = restore }()
	var sawCreate bool
	runBrCommand = func(args ...string) ([]byte, error) {
		if len(args) == 0 {
			t.Fatalf("unexpected br command: %#v", args)
		}
		if args[0] == "list" {
			return []byte(`[]`), nil
		}
		if args[0] != "create" {
			t.Fatalf("unexpected br command: %#v", args)
		}
		sawCreate = true
		if !slices.Contains(args, "--description") {
			t.Fatalf("expected create to include description, got %#v", args)
		}
		return []byte(`{"id":"bead-301","title":"Create issue"}`), nil
	}

	adapter := NewBeadsAdapter()
	task := TaskProjection{
		ID:    "fixture.task.create",
		Title: "Create issue",
		Provenance: TaskProvenance{
			Path:       "testdata/plans/mixed.md",
			StartLine:  3,
			EndLine:    5,
			SourceHash: strings.Repeat("3", 64),
		},
		Acceptance: []string{"cmd:go test ./..."},
	}

	result, err := adapter.PushTask(context.Background(), task)
	if err != nil {
		t.Fatalf("push task: %v", err)
	}
	if !sawCreate {
		t.Fatalf("expected br create to be called")
	}
	if result.RemoteID != "bead-301" {
		t.Fatalf("expected remote id bead-301, got %#v", result)
	}
	manifest := adapter.BuildSyncManifest()
	if len(manifest.Entries) != 1 || manifest.Entries[0].RemoteID != "bead-301" {
		t.Fatalf("expected manifest to store real bead id, got %#v", manifest.Entries)
	}
}

func TestBeadsWriteSyncManifest(t *testing.T) {
	restore := runBrCommand
	defer func() { runBrCommand = restore }()
	runBrCommand = func(args ...string) ([]byte, error) {
		if len(args) == 0 {
			t.Fatalf("unexpected br command: %#v", args)
		}
		if args[0] == "list" {
			return []byte(`[]`), nil
		}
		if args[0] != "create" {
			t.Fatalf("unexpected br command: %#v", args)
		}
		if slices.Contains(args, "Manifest A") {
			return []byte(`{"id":"bead-401","title":"Manifest A"}`), nil
		}
		if slices.Contains(args, "Manifest B") {
			return []byte(`{"id":"bead-402","title":"Manifest B"}`), nil
		}
		t.Fatalf("unexpected br create args: %#v", args)
		return nil, nil
	}

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

func TestBeadsAcceptsBDIssueIDsFromBr(t *testing.T) {
	restore := runBrCommand
	defer func() { runBrCommand = restore }()

	createCalls := 0
	runBrCommand = func(args ...string) ([]byte, error) {
		if len(args) == 0 {
			t.Fatalf("unexpected br command: %#v", args)
		}
		switch args[0] {
		case "list":
			if createCalls == 0 {
				return []byte(`[]`), nil
			}
			return []byte(`[{"id":"bd-13r","title":"Encode the action system","external_ref":"fv.reconcile.actions"}]`), nil
		case "create":
			createCalls++
			return []byte(`{"id":"bd-13r","title":"Encode the action system"}`), nil
		case "update":
			return []byte(`[{"id":"bd-13r","title":"Encode the action system","external_ref":"fv.reconcile.actions"}]`), nil
		default:
			t.Fatalf("unexpected br command: %#v", args)
			return nil, nil
		}
	}

	adapter := NewBeadsAdapter()
	task := TaskProjection{
		ID:    "fv.reconcile.actions",
		Title: "Encode the action system",
		Provenance: TaskProvenance{
			NodeRef:    "./PLAN.md|heading|actions#1",
			Path:       "./PLAN.md",
			StartLine:  102,
			EndLine:    137,
			SourceHash: strings.Repeat("a", 64),
			CompileID:  strings.Repeat("c", 64),
		},
	}

	first, err := adapter.PushTask(context.Background(), task)
	if err != nil {
		t.Fatalf("first push task: %v", err)
	}
	if first.RemoteID != "bd-13r" {
		t.Fatalf("expected first remote id bd-13r, got %#v", first)
	}

	second, err := adapter.PushTask(context.Background(), task)
	if err != nil {
		t.Fatalf("second push task: %v", err)
	}
	if second.RemoteID != "bd-13r" {
		t.Fatalf("expected second remote id bd-13r, got %#v", second)
	}
	if createCalls != 1 {
		t.Fatalf("expected exactly one create call, got %d", createCalls)
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
