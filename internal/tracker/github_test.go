package tracker

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestGitHubAdapterCapabilities(t *testing.T) {
	adapter := NewGitHubAdapter()

	got := adapter.Capabilities()
	if got.AdapterName != "github" {
		t.Fatalf("expected github adapter name, got %#v", got)
	}
	if !got.Title {
		t.Fatalf("expected github to support titles, got %#v", got)
	}
	if got.Body != TextMarkdown {
		t.Fatalf("expected markdown body support, got %#v", got)
	}
	if got.Steps != CapabilityRendered {
		t.Fatalf("expected rendered step support, got %#v", got)
	}
	if got.ChildWork != CapabilityUnsupported {
		t.Fatalf("expected no child-work support, got %#v", got)
	}
	if got.CustomFields != CapabilityUnsupported {
		t.Fatalf("expected no custom-field support, got %#v", got)
	}
	if !got.RuntimeOverlays.Status || !got.RuntimeOverlays.Assignee || got.RuntimeOverlays.Priority {
		t.Fatalf("expected github runtime overlays status+assignee only, got %#v", got)
	}
}

func TestBuildGitHubIssuePayloadRendersMarkdownBody(t *testing.T) {
	task := fixtureRenderedTask()

	payload, err := BuildGitHubIssuePayload(task)
	if err != nil {
		t.Fatalf("build github issue payload: %v", err)
	}

	if payload.ID != task.ID {
		t.Fatalf("expected id %q, got %#v", task.ID, payload)
	}
	if payload.Title != task.Title {
		t.Fatalf("expected title %q, got %#v", task.Title, payload)
	}
	if !strings.Contains(payload.Body, "## Why") || !strings.Contains(payload.Body, "## Steps") {
		t.Fatalf("expected markdown body sections, got %q", payload.Body)
	}
	if !strings.Contains(payload.Body, "- [ ] Write additive migration") {
		t.Fatalf("expected rendered step checklist, got %q", payload.Body)
	}
	if !reflect.DeepEqual(payload.Labels, []string{"horizon:now"}) {
		t.Fatalf("expected deterministic horizon label, got %#v", payload.Labels)
	}
}

func TestGitHubPushIdempotentOnProjectionHash(t *testing.T) {
	adapter := NewGitHubAdapter()
	ctx := context.Background()
	task := fixtureRenderedTask()

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
	if first.RemoteID != "github:"+task.ID || second.RemoteID != first.RemoteID {
		t.Fatalf("expected stable github remote ids, got %q then %q", first.RemoteID, second.RemoteID)
	}
}

func TestGitHubPushNormalizesTaskIDForIdempotence(t *testing.T) {
	adapter := NewGitHubAdapter()
	ctx := context.Background()
	task := fixtureRenderedTask()
	task.ID = "  " + task.ID + "  "

	first, err := adapter.PushTask(ctx, task)
	if err != nil {
		t.Fatalf("first push: %v", err)
	}
	second, err := adapter.PushTask(ctx, task)
	if err != nil {
		t.Fatalf("second push: %v", err)
	}
	if second.Mutated || !second.Noop {
		t.Fatalf("expected second push to be noop after id normalization; got mutated=%v noop=%v", second.Mutated, second.Noop)
	}
	if first.RemoteID != "github:api.migrate" || second.RemoteID != first.RemoteID {
		t.Fatalf("expected normalized github remote ids, got %q then %q", first.RemoteID, second.RemoteID)
	}
}

func TestGitHubPullOnlySafeFields(t *testing.T) {
	adapter := NewGitHubAdapter()
	adapter.SetRemoteRuntimeFields("fixture.task.github", RuntimeFields{
		Status:   "open",
		Assignee: "agent.github",
		Priority: "P1",
	})

	got, err := adapter.PullRuntimeFields(context.Background(), []string{"fixture.task.github"})
	if err != nil {
		t.Fatalf("pull runtime fields: %v", err)
	}
	state, ok := got["fixture.task.github"]
	if !ok {
		t.Fatalf("expected runtime fields for fixture.task.github")
	}
	if state.Status != "open" || state.Assignee != "agent.github" {
		t.Fatalf("unexpected github runtime fields: %#v", state)
	}
	if state.Priority != "" {
		t.Fatalf("expected github adapter to drop unsupported priority, got %#v", state)
	}
}

func TestGitHubPullUsesLastSeenRuntimeHash(t *testing.T) {
	adapter := NewGitHubAdapter()
	id := "fixture.task.github.runtime"

	adapter.SetRemoteRuntimeFields(id, RuntimeFields{
		Status:   "open",
		Assignee: "agent.github",
		Priority: "P1",
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
		Status:   "closed",
		Assignee: "agent.github",
		Priority: "P3",
	})
	third, err := adapter.PullRuntimeFields(context.Background(), []string{id})
	if err != nil {
		t.Fatalf("third pull runtime fields: %v", err)
	}
	updated, ok := third[id]
	if !ok {
		t.Fatalf("expected third pull to return updated runtime fields")
	}
	if updated.Status != "closed" || updated.Priority != "" {
		t.Fatalf("unexpected updated github runtime fields: %#v", updated)
	}
}

func TestGitHubPullNormalizesRequestedIDs(t *testing.T) {
	adapter := NewGitHubAdapter()
	adapter.SetRemoteRuntimeFields("fixture.task.github.trimmed", RuntimeFields{
		Status:   "open",
		Assignee: "agent.github",
	})

	got, err := adapter.PullRuntimeFields(context.Background(), []string{"  fixture.task.github.trimmed  "})
	if err != nil {
		t.Fatalf("pull runtime fields: %v", err)
	}
	if _, ok := got["fixture.task.github.trimmed"]; !ok {
		t.Fatalf("expected normalized runtime lookup key, got %#v", got)
	}
}

func TestGitHubPushClosesDoneTaskAndReopensOpenTask(t *testing.T) {
	adapter := NewGitHubAdapter()
	ctx := context.Background()
	task := fixtureRenderedTask()

	first, err := adapter.PushTask(ctx, task)
	if err != nil {
		t.Fatalf("first push: %v", err)
	}
	if !first.Mutated {
		t.Fatalf("expected first push to mutate")
	}

	doneTask := task
	doneTask.CanonicalStatus = "done"
	closed, err := adapter.PushTask(ctx, doneTask)
	if err != nil {
		t.Fatalf("close done task: %v", err)
	}
	if !closed.Mutated || closed.Noop {
		t.Fatalf("expected canonical done push to mutate tracker state, got %#v", closed)
	}
	if adapter.runtimeByID[strings.TrimSpace(task.ID)].Status != "closed" {
		t.Fatalf("expected github runtime status closed, got %#v", adapter.runtimeByID[strings.TrimSpace(task.ID)])
	}

	reopened, err := adapter.PushTask(ctx, task)
	if err != nil {
		t.Fatalf("reopen open task: %v", err)
	}
	if !reopened.Mutated || reopened.Noop {
		t.Fatalf("expected reopening open task to mutate tracker state, got %#v", reopened)
	}
	if adapter.runtimeByID[strings.TrimSpace(task.ID)].Status != "open" {
		t.Fatalf("expected github runtime status open, got %#v", adapter.runtimeByID[strings.TrimSpace(task.ID)])
	}
}
