package tracker

import (
	"context"
	"strings"
	"testing"
)

func TestBeadsProjectionPayloadContainsHashes(t *testing.T) {
	task := TaskProjection{
		ID:              "fixture.task.now",
		Title:           "Implement deterministic output",
		Anchor:          "testdata/plans/mixed.md#L12",
		SourcePath:      "testdata/plans/mixed.md",
		SourceStartLine: 12,
		SourceEndLine:   14,
		SourceHash:      strings.Repeat("a", 64),
		Accept: []string{
			"cmd:go test ./... -run TestCompile",
			"file:.planmark/tmp/plan.json exists",
		},
	}

	payload, err := BuildProjectionPayload(task)
	if err != nil {
		t.Fatalf("build projection payload: %v", err)
	}

	if payload.ProjectionSchemaVersion != ProjectionSchemaVersionV01 {
		t.Fatalf("expected projection schema version %q, got %q", ProjectionSchemaVersionV01, payload.ProjectionSchemaVersion)
	}
	if payload.ID != task.ID {
		t.Fatalf("expected id %q, got %q", task.ID, payload.ID)
	}
	if payload.Anchor == "" {
		t.Fatalf("expected non-empty anchor")
	}
	if payload.SourceRange.Path != task.SourcePath {
		t.Fatalf("expected source path %q, got %q", task.SourcePath, payload.SourceRange.Path)
	}
	if payload.SourceRange.StartLine != task.SourceStartLine || payload.SourceRange.EndLine != task.SourceEndLine {
		t.Fatalf("expected source range %d-%d, got %d-%d", task.SourceStartLine, task.SourceEndLine, payload.SourceRange.StartLine, payload.SourceRange.EndLine)
	}
	if payload.SourceHash != task.SourceHash {
		t.Fatalf("expected source hash %q, got %q", task.SourceHash, payload.SourceHash)
	}
	if payload.AcceptanceDigest == "" {
		t.Fatalf("expected non-empty acceptance digest")
	}
	if len(payload.AcceptanceDigest) != 64 {
		t.Fatalf("expected sha256 hex digest length 64, got %d", len(payload.AcceptanceDigest))
	}
}

func TestBeadsProjectionPayloadRespectsProjectionVersion(t *testing.T) {
	task := TaskProjection{
		ID:                "fixture.task.version",
		Title:             "Projection version passthrough",
		SourcePath:        "testdata/plans/mixed.md",
		SourceStartLine:   3,
		SourceEndLine:     4,
		SourceHash:        strings.Repeat("c", 64),
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

func TestBeadsPushIdempotentOnProjectionHash(t *testing.T) {
	adapter := NewBeadsAdapter()
	ctx := context.Background()

	task := TaskProjection{
		ID:              "fixture.task.idempotent",
		Title:           "Idempotent push check",
		SourcePath:      "testdata/plans/mixed.md",
		SourceStartLine: 7,
		SourceEndLine:   9,
		SourceHash:      strings.Repeat("b", 64),
		Accept:          []string{"cmd:go test ./... -run TestCompile"},
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
