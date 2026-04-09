package tracker

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestLinearAdapterCapabilities(t *testing.T) {
	adapter := NewLinearAdapter()

	got := adapter.Capabilities()
	if got.AdapterName != "linear" {
		t.Fatalf("expected linear adapter name, got %#v", got)
	}
	if !got.Title {
		t.Fatalf("expected linear to support titles, got %#v", got)
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
		t.Fatalf("expected linear runtime overlays status+assignee only, got %#v", got)
	}
}

func TestBuildLinearIssuePayloadRendersMarkdownBody(t *testing.T) {
	task := fixtureRenderedTask()

	payload, err := BuildLinearIssuePayload(task)
	if err != nil {
		t.Fatalf("build linear issue payload: %v", err)
	}

	if payload.ID != task.ID {
		t.Fatalf("expected id %q, got %#v", task.ID, payload)
	}
	if payload.Title != task.Title {
		t.Fatalf("expected title %q, got %#v", task.Title, payload)
	}
	if !strings.Contains(payload.Description, "## Why") || !strings.Contains(payload.Description, "## Steps") {
		t.Fatalf("expected markdown body sections, got %q", payload.Description)
	}
	if !strings.Contains(payload.Description, "- [ ] Write additive migration") {
		t.Fatalf("expected rendered step checklist, got %q", payload.Description)
	}
	if !reflect.DeepEqual(payload.Labels, []string{"horizon:now"}) {
		t.Fatalf("expected deterministic horizon label, got %#v", payload.Labels)
	}
}

func TestLinearPushIdempotentOnProjectionHash(t *testing.T) {
	adapter := NewLinearAdapter()
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
	if first.RemoteID != "linear:"+task.ID || second.RemoteID != first.RemoteID {
		t.Fatalf("expected stable linear remote ids, got %q then %q", first.RemoteID, second.RemoteID)
	}
}

func TestLinearPushNormalizesTaskIDForIdempotence(t *testing.T) {
	adapter := NewLinearAdapter()
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
	if first.RemoteID != "linear:api.migrate" || second.RemoteID != first.RemoteID {
		t.Fatalf("expected normalized linear remote ids, got %q then %q", first.RemoteID, second.RemoteID)
	}
}

func TestLinearPullOnlySafeFields(t *testing.T) {
	adapter := NewLinearAdapter()
	adapter.SetRemoteRuntimeFields("fixture.task.linear", RuntimeFields{
		Status:   "unstarted",
		Assignee: "agent.linear",
		Priority: "P1",
	})

	got, err := adapter.PullRuntimeFields(context.Background(), []string{"fixture.task.linear"})
	if err != nil {
		t.Fatalf("pull runtime fields: %v", err)
	}
	state, ok := got["fixture.task.linear"]
	if !ok {
		t.Fatalf("expected runtime fields for fixture.task.linear")
	}
	if state.Status != "unstarted" || state.Assignee != "agent.linear" {
		t.Fatalf("unexpected linear runtime fields: %#v", state)
	}
	if state.Priority != "" {
		t.Fatalf("expected linear adapter to drop unsupported priority, got %#v", state)
	}
}

func TestLinearPullNormalizesRequestedIDs(t *testing.T) {
	adapter := NewLinearAdapter()
	adapter.SetRemoteRuntimeFields("fixture.task.linear.trimmed", RuntimeFields{
		Status:   "open",
		Assignee: "agent.linear",
	})

	got, err := adapter.PullRuntimeFields(context.Background(), []string{"  fixture.task.linear.trimmed  "})
	if err != nil {
		t.Fatalf("pull runtime fields: %v", err)
	}
	if _, ok := got["fixture.task.linear.trimmed"]; !ok {
		t.Fatalf("expected normalized runtime lookup key, got %#v", got)
	}
}

func TestLinearPushClosesDoneTaskAndReopensOpenTask(t *testing.T) {
	adapter := NewLinearAdapter()
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
		t.Fatalf("expected linear runtime status closed, got %#v", adapter.runtimeByID[strings.TrimSpace(task.ID)])
	}

	reopened, err := adapter.PushTask(ctx, task)
	if err != nil {
		t.Fatalf("reopen open task: %v", err)
	}
	if !reopened.Mutated || reopened.Noop {
		t.Fatalf("expected reopening open task to mutate tracker state, got %#v", reopened)
	}
	if adapter.runtimeByID[strings.TrimSpace(task.ID)].Status != "open" {
		t.Fatalf("expected linear runtime status open, got %#v", adapter.runtimeByID[strings.TrimSpace(task.ID)])
	}
}
