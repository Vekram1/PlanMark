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
		ID:              "fixture.task.now",
		Title:           "Implement deterministic output",
		CanonicalStatus: "done",
		Horizon:         "now",
		Anchor:          "testdata/plans/mixed.md#L12",
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

	if payload.ProjectionSchemaVersion != ProjectionSchemaVersionV03 {
		t.Fatalf("expected projection schema version %q, got %q", ProjectionSchemaVersionV03, payload.ProjectionSchemaVersion)
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
	if payload.CanonicalStatus != task.CanonicalStatus {
		t.Fatalf("expected canonical status %q, got %q", task.CanonicalStatus, payload.CanonicalStatus)
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
		case "show":
			return []byte(`[{"id":"bead-101","status":"open"}]`), nil
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

func TestBeadsPushReopensClosedIssueWithUnchangedProjection(t *testing.T) {
	restore := runBrCommand
	defer func() { runBrCommand = restore }()
	seen := make([][]string, 0, 4)
	runBrCommand = func(args ...string) ([]byte, error) {
		seen = append(seen, append([]string(nil), args...))
		switch args[0] {
		case "show":
			return []byte(`[{"id":"bd-101","status":"closed"}]`), nil
		case "reopen":
			return []byte(`[{"id":"bd-101","status":"open"}]`), nil
		case "update":
			return []byte(`[{"id":"bd-101","status":"open","external_ref":"pm.context.root"}]`), nil
		default:
			t.Fatalf("unexpected br command: %#v", args)
			return nil, nil
		}
	}

	adapter := NewBeadsAdapter()
	ctx := context.Background()
	task := TaskProjection{
		ID:    "pm.context.root",
		Title: "Replace level-based context with need-based retrieval",
		Provenance: TaskProvenance{
			Path:       "PLAN.md",
			StartLine:  1,
			EndLine:    8,
			SourceHash: strings.Repeat("a", 64),
		},
		Acceptance: []string{"cmd:test -f docs/specs/context-selection-v0.1.md"},
	}
	hash, err := TaskProjectionHash(task)
	if err != nil {
		t.Fatalf("projection hash: %v", err)
	}
	adapter.remoteIDByID[task.ID] = "bd-101"
	adapter.projectionHashByID[task.ID] = hash
	adapter.sourceHashByID[task.ID] = task.Provenance.SourceHash
	adapter.provenanceByID[task.ID] = task.Provenance

	result, err := adapter.PushTask(ctx, task)
	if err != nil {
		t.Fatalf("push task: %v", err)
	}
	if !result.Mutated || !strings.Contains(result.Diagnostic, "reopened closed tracker issue") {
		t.Fatalf("expected closed issue reopen mutation, got %#v", result)
	}
	if len(seen) != 3 || seen[0][0] != "show" || seen[1][0] != "reopen" || seen[2][0] != "update" {
		t.Fatalf("expected show->reopen->update flow, got %#v", seen)
	}
}

func TestBeadsPushClosesCanonicallyDoneIssueWithUnchangedProjection(t *testing.T) {
	restore := runBrCommand
	defer func() { runBrCommand = restore }()
	seen := make([][]string, 0, 3)
	runBrCommand = func(args ...string) ([]byte, error) {
		seen = append(seen, append([]string(nil), args...))
		switch args[0] {
		case "show":
			return []byte(`[{"id":"bd-202","status":"open"}]`), nil
		case "dep":
			if len(args) >= 2 && args[1] == "list" {
				return []byte(`[]`), nil
			}
			t.Fatalf("unexpected dep command: %#v", args)
			return nil, nil
		case "close":
			return []byte(`[{"id":"bd-202","status":"closed"}]`), nil
		default:
			t.Fatalf("unexpected br command: %#v", args)
			return nil, nil
		}
	}

	adapter := NewBeadsAdapter()
	task := TaskProjection{
		ID:              "pm.context.eval",
		Title:           "Add evaluation and telemetry for context sufficiency",
		CanonicalStatus: "done",
		Provenance: TaskProvenance{
			Path:       "PLAN.md",
			StartLine:  1,
			EndLine:    8,
			SourceHash: strings.Repeat("d", 64),
		},
		Acceptance: []string{"cmd:test -f docs/specs/context-selection-v0.1.md"},
	}
	hash, err := TaskProjectionHash(task)
	if err != nil {
		t.Fatalf("projection hash: %v", err)
	}
	adapter.remoteIDByID[task.ID] = "bd-202"
	adapter.projectionHashByID[task.ID] = hash
	adapter.sourceHashByID[task.ID] = task.Provenance.SourceHash
	adapter.provenanceByID[task.ID] = task.Provenance

	result, err := adapter.PushTask(context.Background(), task)
	if err != nil {
		t.Fatalf("push task: %v", err)
	}
	if !result.Mutated || !strings.Contains(result.Diagnostic, "canonically completed") {
		t.Fatalf("expected canonical completion close mutation, got %#v", result)
	}
	if len(seen) != 3 || seen[0][0] != "show" || seen[1][0] != "dep" || seen[1][1] != "list" || seen[2][0] != "close" {
		t.Fatalf("expected show->dep list->close flow, got %#v", seen)
	}
}

func TestBeadsPushClosesCanonicallyDoneIssueAfterRemovingDependencies(t *testing.T) {
	restore := runBrCommand
	defer func() { runBrCommand = restore }()

	var seen [][]string
	runBrCommand = func(args ...string) ([]byte, error) {
		seen = append(seen, append([]string(nil), args...))
		switch {
		case reflect.DeepEqual(args, []string{"show", "bd-202", "--json"}):
			return []byte(`[{"id":"bd-202","title":"Add evaluation and telemetry for context sufficiency","status":"open","external_ref":"pm.context.eval"}]`), nil
		case reflect.DeepEqual(args, []string{"dep", "list", "bd-202", "--json"}):
			return []byte(`[{"issue_id":"bd-202","depends_on_id":"bd-parent","type":"blocks"}]`), nil
		case reflect.DeepEqual(args, []string{"dep", "remove", "bd-202", "bd-parent", "--json"}):
			return []byte(`{"ok":true}`), nil
		case len(args) >= 2 && args[0] == "close" && args[1] == "bd-202":
			return []byte(`[{"id":"bd-202","status":"closed"}]`), nil
		default:
			t.Fatalf("unexpected br command: %#v", args)
			return nil, nil
		}
	}

	adapter := NewBeadsAdapter()
	task := TaskProjection{
		ID:              "pm.context.eval",
		Title:           "Add evaluation and telemetry for context sufficiency",
		CanonicalStatus: "done",
		Provenance: TaskProvenance{
			Path:       "PLAN.md",
			StartLine:  1,
			EndLine:    8,
			SourceHash: strings.Repeat("d", 64),
		},
		Acceptance: []string{"cmd:test -f docs/specs/context-selection-v0.1.md"},
	}
	hash, err := TaskProjectionHash(task)
	if err != nil {
		t.Fatalf("projection hash: %v", err)
	}
	adapter.remoteIDByID[task.ID] = "bd-202"
	adapter.projectionHashByID[task.ID] = hash
	adapter.sourceHashByID[task.ID] = task.Provenance.SourceHash
	adapter.provenanceByID[task.ID] = task.Provenance

	result, err := adapter.PushTask(context.Background(), task)
	if err != nil {
		t.Fatalf("push task: %v", err)
	}
	if !result.Mutated || !strings.Contains(result.Diagnostic, "canonically completed") {
		t.Fatalf("expected canonical completion close mutation, got %#v", result)
	}
	want := [][]string{
		{"show", "bd-202", "--json"},
		{"dep", "list", "bd-202", "--json"},
		{"dep", "remove", "bd-202", "bd-parent", "--json"},
	}
	if len(seen) != 4 || !reflect.DeepEqual(seen[:3], want) || seen[3][0] != "close" || seen[3][1] != "bd-202" {
		t.Fatalf("expected show->dep list->dep remove->close flow, got %#v", seen)
	}
}

func TestBeadsMarkTaskStaleTreatsAlreadyClosedIssueAsNoop(t *testing.T) {
	restore := runBrCommand
	defer func() { runBrCommand = restore }()
	var seen [][]string
	runBrCommand = func(args ...string) ([]byte, error) {
		seen = append(seen, append([]string(nil), args...))
		if len(args) >= 2 && args[0] == "dep" && args[1] == "list" {
			return []byte(`[]`), nil
		}
		if len(args) >= 1 && args[0] == "close" {
			return nil, errors.New(`exit status 3: []
{
  "error": {
    "code": "NOTHING_TO_DO",
    "message": "Nothing to do: all 1 issue(s) skipped"
  }
}`)
		}
		t.Fatalf("unexpected br command: %#v", args)
		return nil, nil
	}

	adapter := NewBeadsAdapter()
	adapter.remoteIDByID["fixture.task.stale"] = "bead-101"
	adapter.projectionHashByID["fixture.task.stale"] = "old-hash"
	adapter.sourceHashByID["fixture.task.stale"] = strings.Repeat("a", 64)

	result, err := adapter.MarkTaskStale(context.Background(), "fixture.task.stale", "missing in desired")
	if err != nil {
		t.Fatalf("mark task stale: %v", err)
	}
	if !result.Noop || result.Mutated {
		t.Fatalf("expected already-closed stale task to be noop, got %#v", result)
	}
	if !strings.Contains(result.Diagnostic, "already closed or absent") {
		t.Fatalf("expected noop diagnostic, got %#v", result)
	}
	if _, ok := adapter.remoteIDByID["fixture.task.stale"]; ok {
		t.Fatalf("expected stale task state to be dropped after noop close")
	}
	if len(seen) != 2 || seen[0][0] != "dep" || seen[0][1] != "list" || seen[1][0] != "close" {
		t.Fatalf("expected dep list -> close flow, got %#v", seen)
	}
}

func TestBeadsListCleanupCandidatesFindsExternalRefAndForeignPlanIssues(t *testing.T) {
	restore := runBrCommand
	defer func() { runBrCommand = restore }()
	runBrCommand = func(args ...string) ([]byte, error) {
		if !reflect.DeepEqual(args, []string{"list", "--status", "open", "--json"}) {
			t.Fatalf("unexpected br command: %#v", args)
		}
		return []byte(`[
			{"id":"bd-1","title":"Current plan task","external_ref":"pm.context.root","description":"## Provenance\n- source: ./PLAN.md:1-4"},
			{"id":"bd-2","title":"Removed plan task","external_ref":"old.task","description":"## Provenance\n- source: ./PLAN.md:5-8"},
			{"id":"bd-3","title":"Temp test task","description":"## Provenance\n- source: /tmp/test/PLAN.md:1-3"},
			{"id":"bd-4","title":"Manual issue","description":"No provenance here"}
		]`), nil
	}

	adapter := NewBeadsAdapter()
	candidates, err := adapter.ListCleanupCandidates(context.Background(), "./PLAN.md", map[string]struct{}{
		"pm.context.root": {},
	})
	if err != nil {
		t.Fatalf("list cleanup candidates: %v", err)
	}
	want := []CleanupCandidate{
		{RemoteID: "bd-2", Title: "Removed plan task", ExternalRef: "old.task", Reason: "external_ref is missing from current plan"},
		{RemoteID: "bd-3", Title: "Temp test task", SourcePath: "/tmp/test/PLAN.md", Reason: "plan-derived issue points at a different plan file"},
	}
	if !reflect.DeepEqual(candidates, want) {
		t.Fatalf("unexpected cleanup candidates\nwant=%#v\ngot=%#v", want, candidates)
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
		case "show":
			return []byte(`[{"id":"bead-201","status":"open"}]`), nil
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
		descriptionIndex := slices.Index(args, "--description")
		if descriptionIndex < 0 || descriptionIndex+1 >= len(args) {
			t.Fatalf("expected create description argument, got %#v", args)
		}
		if strings.Count(args[descriptionIndex+1], "## Acceptance") != 1 {
			t.Fatalf("expected create description to render acceptance once, got %#v", args)
		}
		if !slices.Contains(args, "--priority") || !slices.Contains(args, "1") {
			t.Fatalf("expected create to include mapped priority 1, got %#v", args)
		}
		return []byte(`{"id":"bead-301","title":"Create issue"}`), nil
	}

	adapter := NewBeadsAdapter()
	task := TaskProjection{
		ID:      "fixture.task.create",
		Title:   "Create issue",
		Horizon: "now",
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

func TestBeadsPushUpdatesPriorityFromHorizon(t *testing.T) {
	restore := runBrCommand
	defer func() { runBrCommand = restore }()
	seen := make([][]string, 0, 4)
	runBrCommand = func(args ...string) ([]byte, error) {
		seen = append(seen, append([]string(nil), args...))
		switch args[0] {
		case "list":
			return []byte(`[]`), nil
		case "show":
			return []byte(`[{"id":"bead-410","status":"open","external_ref":"fixture.task.update.priority"}]`), nil
		case "create":
			return []byte(`{"id":"bead-410","title":"Update priority"}`), nil
		case "update":
			return []byte(`[{"id":"bead-410","status":"open","external_ref":"fixture.task.update.priority"}]`), nil
		default:
			t.Fatalf("unexpected br command: %#v", args)
			return nil, nil
		}
	}

	adapter := NewBeadsAdapter()
	task := TaskProjection{
		ID:      "fixture.task.update.priority",
		Title:   "Update priority",
		Horizon: "now",
		Provenance: TaskProvenance{
			Path:       "testdata/plans/mixed.md",
			StartLine:  3,
			EndLine:    5,
			SourceHash: strings.Repeat("4", 64),
		},
	}
	if _, err := adapter.PushTask(context.Background(), task); err != nil {
		t.Fatalf("first push: %v", err)
	}

	task.Horizon = "next"
	task.Title = "Update priority again"
	if _, err := adapter.PushTask(context.Background(), task); err != nil {
		t.Fatalf("second push: %v", err)
	}

	foundUpdate := false
	for _, args := range seen {
		if len(args) > 0 && args[0] == "update" {
			foundUpdate = true
			if !slices.Contains(args, "--priority") || !slices.Contains(args, "2") {
				t.Fatalf("expected update to include mapped priority 2, got %#v", args)
			}
		}
	}
	if !foundUpdate {
		t.Fatalf("expected an update call, got %#v", seen)
	}
}

func TestBeadsSyncDependenciesAddsMissingAndRemovesStaleEdges(t *testing.T) {
	restore := runBrCommand
	defer func() { runBrCommand = restore }()

	seen := make([][]string, 0, 8)
	runBrCommand = func(args ...string) ([]byte, error) {
		seen = append(seen, append([]string(nil), args...))
		switch {
		case reflect.DeepEqual(args, []string{"dep", "list", "bead-root", "--json"}):
			return []byte(`[
				{"issue_id":"bead-root","depends_on_id":"bead-old","type":"blocks"},
				{"issue_id":"bead-root","depends_on_id":"bead-keep","type":"related"}
			]`), nil
		case reflect.DeepEqual(args, []string{"dep", "list", "bead-dep", "--json"}):
			return []byte(`[]`), nil
		case reflect.DeepEqual(args, []string{"dep", "add", "bead-root", "bead-dep", "--json"}):
			return []byte(`{"ok":true}`), nil
		case reflect.DeepEqual(args, []string{"dep", "remove", "bead-root", "bead-old", "--json"}):
			return []byte(`{"ok":true}`), nil
		default:
			t.Fatalf("unexpected br command: %#v", args)
			return nil, nil
		}
	}

	adapter := NewBeadsAdapter()
	adapter.remoteIDByID["fixture.task.root"] = "bead-root"
	adapter.remoteIDByID["fixture.task.dep"] = "bead-dep"

	err := adapter.SyncDependencies(context.Background(), map[string]TaskProjection{
		"fixture.task.root": {
			ID:           "fixture.task.root",
			Title:        "Root",
			Dependencies: []string{"fixture.task.dep"},
		},
		"fixture.task.dep": {
			ID:    "fixture.task.dep",
			Title: "Dep",
		},
	})
	if err != nil {
		t.Fatalf("sync dependencies: %v", err)
	}

	wantCommands := [][]string{
		{"dep", "list", "bead-dep", "--json"},
		{"dep", "list", "bead-root", "--json"},
		{"dep", "add", "bead-root", "bead-dep", "--json"},
		{"dep", "remove", "bead-root", "bead-old", "--json"},
	}
	if !reflect.DeepEqual(seen, wantCommands) {
		t.Fatalf("unexpected dependency reconciliation commands\nwant=%#v\ngot=%#v", wantCommands, seen)
	}
}

func TestBeadsSyncDependenciesResolvesDependencyRemoteIDByExternalRef(t *testing.T) {
	restore := runBrCommand
	defer func() { runBrCommand = restore }()

	runBrCommand = func(args ...string) ([]byte, error) {
		switch args[0] {
		case "list":
			return []byte(`[
				{"id":"bead-dep","external_ref":"fixture.task.dep","title":"Dependency"}
			]`), nil
		case "dep":
			if reflect.DeepEqual(args, []string{"dep", "list", "bead-root", "--json"}) {
				return []byte(`[]`), nil
			}
			if reflect.DeepEqual(args, []string{"dep", "add", "bead-root", "bead-dep", "--json"}) {
				return []byte(`{"ok":true}`), nil
			}
			t.Fatalf("unexpected br dep command: %#v", args)
			return nil, nil
		default:
			t.Fatalf("unexpected br command: %#v", args)
			return nil, nil
		}
	}

	adapter := NewBeadsAdapter()
	adapter.remoteIDByID["fixture.task.root"] = "bead-root"

	err := adapter.SyncDependencies(context.Background(), map[string]TaskProjection{
		"fixture.task.root": {
			ID:           "fixture.task.root",
			Title:        "Root",
			Dependencies: []string{"fixture.task.dep"},
		},
		"fixture.task.dep": {
			ID:    "fixture.task.dep",
			Title: "Dep",
		},
	})
	if err != nil {
		t.Fatalf("sync dependencies: %v", err)
	}
	if adapter.remoteIDByID["fixture.task.dep"] != "bead-dep" {
		t.Fatalf("expected dependency remote id to be cached, got %#v", adapter.remoteIDByID)
	}
}

func TestBeadsSyncDependenciesIgnoresMissingPlanDepsAndRemovesStaleEdges(t *testing.T) {
	restore := runBrCommand
	defer func() { runBrCommand = restore }()

	seen := make([][]string, 0, 8)
	runBrCommand = func(args ...string) ([]byte, error) {
		seen = append(seen, append([]string(nil), args...))
		switch {
		case reflect.DeepEqual(args, []string{"dep", "list", "bead-root", "--json"}):
			return []byte(`[
				{"issue_id":"bead-root","depends_on_id":"bead-missing","type":"blocks"}
			]`), nil
		case reflect.DeepEqual(args, []string{"dep", "remove", "bead-root", "bead-missing", "--json"}):
			return []byte(`{"ok":true}`), nil
		default:
			t.Fatalf("unexpected br command: %#v", args)
			return nil, nil
		}
	}

	adapter := NewBeadsAdapter()
	adapter.remoteIDByID["fixture.task.root"] = "bead-root"

	err := adapter.SyncDependencies(context.Background(), map[string]TaskProjection{
		"fixture.task.root": {
			ID:           "fixture.task.root",
			Title:        "Root",
			Dependencies: []string{"fixture.task.missing"},
		},
	})
	if err != nil {
		t.Fatalf("sync dependencies: %v", err)
	}

	wantCommands := [][]string{
		{"dep", "list", "bead-root", "--json"},
		{"dep", "remove", "bead-root", "bead-missing", "--json"},
	}
	if !reflect.DeepEqual(seen, wantCommands) {
		t.Fatalf("unexpected dependency reconciliation commands\nwant=%#v\ngot=%#v", wantCommands, seen)
	}
	if _, ok := adapter.remoteIDByID["fixture.task.missing"]; ok {
		t.Fatalf("expected missing plan dependency to avoid tracker identity lookup, got %#v", adapter.remoteIDByID)
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
		case "show":
			return []byte(`[{"id":"bd-13r","title":"Encode the action system","status":"open","external_ref":"fv.reconcile.actions"}]`), nil
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

func TestBeadsAcceptsCustomPrefixIssueIDsFromBr(t *testing.T) {
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
			return []byte(`[{"id":"smoke-swv","title":"Custom prefix issue","external_ref":"smoke.custom.prefix"}]`), nil
		case "show":
			return []byte(`[{"id":"smoke-swv","title":"Custom prefix issue","status":"open","external_ref":"smoke.custom.prefix"}]`), nil
		case "create":
			createCalls++
			return []byte(`{"id":"smoke-swv","title":"Custom prefix issue"}`), nil
		case "update":
			return []byte(`[{"id":"smoke-swv","title":"Custom prefix issue","external_ref":"smoke.custom.prefix"}]`), nil
		default:
			t.Fatalf("unexpected br command: %#v", args)
			return nil, nil
		}
	}

	adapter := NewBeadsAdapter()
	task := TaskProjection{
		ID:    "smoke.custom.prefix",
		Title: "Custom prefix issue",
		Provenance: TaskProvenance{
			NodeRef:    "./PLAN.md|checkbox|custom#1",
			Path:       "./PLAN.md",
			StartLine:  10,
			EndLine:    14,
			SourceHash: strings.Repeat("d", 64),
			CompileID:  strings.Repeat("e", 64),
		},
	}

	first, err := adapter.PushTask(context.Background(), task)
	if err != nil {
		t.Fatalf("first push task: %v", err)
	}
	if first.RemoteID != "smoke-swv" {
		t.Fatalf("expected first remote id smoke-swv, got %#v", first)
	}

	second, err := adapter.PushTask(context.Background(), task)
	if err != nil {
		t.Fatalf("second push task: %v", err)
	}
	if second.RemoteID != "smoke-swv" {
		t.Fatalf("expected second remote id smoke-swv, got %#v", second)
	}
	if createCalls != 1 {
		t.Fatalf("expected exactly one create call, got %d", createCalls)
	}
}

func TestBeadsPushTitleChangeUpdatesExistingIssue(t *testing.T) {
	restore := runBrCommand
	defer func() { runBrCommand = restore }()

	createCalls := 0
	updateCalls := 0
	runBrCommand = func(args ...string) ([]byte, error) {
		if len(args) == 0 {
			t.Fatalf("unexpected br command: %#v", args)
		}
		switch args[0] {
		case "list":
			if createCalls == 0 {
				return []byte(`[]`), nil
			}
			return []byte(`[{"id":"bd-2e7","title":"Verification objective","external_ref":"fv.reconcile.root"}]`), nil
		case "show":
			return []byte(`[{"id":"bd-2e7","title":"Verification objective","status":"open","external_ref":"fv.reconcile.root"}]`), nil
		case "create":
			createCalls++
			if !slices.Contains(args, "--title") || !slices.Contains(args, "Verification objective") {
				t.Fatalf("expected create to use initial title, got %#v", args)
			}
			return []byte(`{"id":"bd-2e7","title":"Verification objective"}`), nil
		case "update":
			updateCalls++
			if !slices.Contains(args, "--title") || !slices.Contains(args, "Verification objectives") {
				t.Fatalf("expected update to use new title, got %#v", args)
			}
			return []byte(`[{"id":"bd-2e7","title":"Verification objectives","external_ref":"fv.reconcile.root"}]`), nil
		default:
			t.Fatalf("unexpected br command: %#v", args)
			return nil, nil
		}
	}

	adapter := NewBeadsAdapter()
	task := TaskProjection{
		ID:    "fv.reconcile.root",
		Title: "Verification objective",
		Provenance: TaskProvenance{
			NodeRef:    "./PLAN.md|heading|root#1",
			Path:       "./PLAN.md",
			StartLine:  5,
			EndLine:    21,
			SourceHash: strings.Repeat("a", 64),
			CompileID:  strings.Repeat("c", 64),
		},
	}

	if _, err := adapter.PushTask(context.Background(), task); err != nil {
		t.Fatalf("first push task: %v", err)
	}

	task.Title = "Verification objectives"
	if _, err := adapter.PushTask(context.Background(), task); err != nil {
		t.Fatalf("second push task: %v", err)
	}

	if createCalls != 1 {
		t.Fatalf("expected exactly one create call, got %d", createCalls)
	}
	if updateCalls != 1 {
		t.Fatalf("expected exactly one update call, got %d", updateCalls)
	}
}

func TestBeadsMarkTaskStaleClosesIssueAndDropsState(t *testing.T) {
	restore := runBrCommand
	defer func() { runBrCommand = restore }()

	var seen [][]string
	runBrCommand = func(args ...string) ([]byte, error) {
		seen = append(seen, append([]string(nil), args...))
		switch args[0] {
		case "dep":
			if len(args) >= 2 && args[1] == "list" {
				return []byte(`[]`), nil
			}
			t.Fatalf("unexpected dep command: %#v", args)
			return nil, nil
		case "close":
			return []byte(`[{"id":"bd-2qn","status":"closed"}]`), nil
		default:
			t.Fatalf("unexpected br command: %#v", args)
			return nil, nil
		}
	}

	adapter := NewBeadsAdapter()
	adapter.SeedFromSyncManifest(SyncManifest{
		SchemaVersion: SyncManifestSchemaVersionV01,
		Entries: []SyncManifestEntry{
			{
				ID:              "fv.reconcile.actions",
				RemoteID:        "bd-2qn",
				ProjectionHash:  "hash-1",
				NodeRef:         "./PLAN.md|heading|actions#1",
				SourcePath:      "./PLAN.md",
				SourceStartLine: 102,
				SourceEndLine:   137,
				SourceHash:      strings.Repeat("a", 64),
				CompileID:       strings.Repeat("c", 64),
			},
		},
	})

	result, err := adapter.MarkTaskStale(context.Background(), "fv.reconcile.actions", "present in prior projection set but missing in desired")
	if err != nil {
		t.Fatalf("mark task stale: %v", err)
	}
	if !result.Mutated || result.RemoteID != "bd-2qn" {
		t.Fatalf("expected stale close mutation, got %#v", result)
	}
	if len(seen) != 2 || seen[0][0] != "dep" || seen[0][1] != "list" || seen[1][0] != "close" || seen[1][1] != "bd-2qn" {
		t.Fatalf("expected dep list -> close call for bd-2qn, got %#v", seen)
	}
	if !slices.Contains(seen[1], "--reason") {
		t.Fatalf("expected close reason in command args, got %#v", seen[1])
	}
	if len(adapter.BuildSyncManifest().Entries) != 0 {
		t.Fatalf("expected closed stale task to be removed from manifest state, got %#v", adapter.BuildSyncManifest().Entries)
	}
}

func TestBeadsMarkTaskStaleRemovesDependenciesBeforeClose(t *testing.T) {
	restore := runBrCommand
	defer func() { runBrCommand = restore }()

	var seen [][]string
	runBrCommand = func(args ...string) ([]byte, error) {
		seen = append(seen, append([]string(nil), args...))
		switch args[0] {
		case "dep":
			switch args[1] {
			case "list":
				return []byte(`[{"issue_id":"bd-2qn","depends_on_id":"bd-dep","type":"blocks"}]`), nil
			case "remove":
				return []byte(`[]`), nil
			default:
				t.Fatalf("unexpected dep subcommand: %#v", args)
			}
		case "close":
			return []byte(`[{"id":"bd-2qn","status":"closed"}]`), nil
		default:
			t.Fatalf("unexpected br command: %#v", args)
		}
		return nil, nil
	}

	adapter := NewBeadsAdapter()
	adapter.SeedFromSyncManifest(SyncManifest{
		SchemaVersion: SyncManifestSchemaVersionV01,
		Entries: []SyncManifestEntry{
			{
				ID:       "fixture.task.dep-close",
				RemoteID: "bd-2qn",
			},
		},
	})

	result, err := adapter.MarkTaskStale(context.Background(), "fixture.task.dep-close", "present in prior projection set but missing in desired")
	if err != nil {
		t.Fatalf("mark task stale: %v", err)
	}
	if !result.Mutated || result.RemoteID != "bd-2qn" {
		t.Fatalf("expected stale close mutation, got %#v", result)
	}
	if len(seen) != 3 {
		t.Fatalf("expected dep list + dep remove + close flow, got %#v", seen)
	}
	if seen[0][0] != "dep" || seen[0][1] != "list" || seen[0][2] != "bd-2qn" {
		t.Fatalf("expected first call to list dependencies, got %#v", seen[0])
	}
	if seen[1][0] != "dep" || seen[1][1] != "remove" || seen[1][2] != "bd-2qn" || seen[1][3] != "bd-dep" {
		t.Fatalf("expected second call to remove dependency, got %#v", seen[1])
	}
	if seen[2][0] != "close" || seen[2][1] != "bd-2qn" {
		t.Fatalf("expected final close call, got %#v", seen[2])
	}
}

func TestBeadsAdapterUsesExplicitDBPath(t *testing.T) {
	restore := runBrCommand
	defer func() { runBrCommand = restore }()

	var seen [][]string
	runBrCommand = func(args ...string) ([]byte, error) {
		seen = append(seen, append([]string(nil), args...))
		switch args[0] {
		case "list":
			return []byte(`[]`), nil
		case "create":
			return []byte(`{"id":"bd-301","title":"Create issue"}`), nil
		default:
			t.Fatalf("unexpected br command: %#v", args)
			return nil, nil
		}
	}

	adapter := NewBeadsAdapter()
	beadsDir := filepath.Join(t.TempDir(), ".beads")
	adapter.SetDBPath(filepath.Join(beadsDir, "beads.db"))
	task := TaskProjection{
		ID:    "fixture.task.create",
		Title: "Create issue",
		Provenance: TaskProvenance{
			Path:       "testdata/plans/mixed.md",
			StartLine:  3,
			EndLine:    5,
			SourceHash: strings.Repeat("3", 64),
		},
	}

	if _, err := adapter.PushTask(context.Background(), task); err != nil {
		t.Fatalf("push task: %v", err)
	}
	if len(seen) != 2 {
		t.Fatalf("expected list + create calls, got %#v", seen)
	}
	for _, args := range seen {
		if len(args) < 3 || args[1] != "--db" {
			t.Fatalf("expected explicit --db arg, got %#v", args)
		}
	}

	metadataRaw, err := os.ReadFile(filepath.Join(beadsDir, "metadata.json"))
	if err != nil {
		t.Fatalf("read generated metadata.json: %v", err)
	}
	var metadata struct {
		Database    string `json:"database"`
		JSONLExport string `json:"jsonl_export"`
	}
	if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
		t.Fatalf("decode metadata.json: %v raw=%q", err, string(metadataRaw))
	}
	if metadata.Database != "beads.db" || metadata.JSONLExport != "issues.jsonl" {
		t.Fatalf("unexpected generated metadata: %#v", metadata)
	}
}

func TestRunBrCommandIgnoresStderrLogsOnSuccess(t *testing.T) {
	tempDir := t.TempDir()
	scriptPath := filepath.Join(tempDir, "br")
	script := strings.Join([]string{
		"#!/bin/sh",
		"echo '{\"id\":\"bd-999\",\"title\":\"stderr noise test\"}'",
		"echo '2026-04-06T21:55:15Z INFO beads_rust::sync: Auto-flush complete' >&2",
	}, "\n")
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake br: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tempDir+string(os.PathListSeparator)+originalPath)

	output, err := runBrCommand("create", "--json")
	if err != nil {
		t.Fatalf("runBrCommand: %v", err)
	}
	if strings.Contains(string(output), "Auto-flush complete") {
		t.Fatalf("expected stdout json only, got %q", string(output))
	}

	var issue beadsIssue
	if err := json.Unmarshal(output, &issue); err != nil {
		t.Fatalf("decode fake br json: %v output=%q", err, string(output))
	}
	if issue.ID != "bd-999" {
		t.Fatalf("expected fake issue id bd-999, got %#v", issue)
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
