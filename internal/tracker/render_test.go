package tracker

import (
	"reflect"
	"strings"
	"testing"
)

func TestRenderTaskDefaultProfileForBeadsCapabilities(t *testing.T) {
	task := fixtureRenderedTask()

	rendered, err := RenderTask(task, NewBeadsAdapter().Capabilities(), RenderProfileDefault)
	if err != nil {
		t.Fatalf("render default profile: %v", err)
	}

	if rendered.Profile != RenderProfileDefault {
		t.Fatalf("expected default profile, got %#v", rendered)
	}
	if rendered.Title != task.Title {
		t.Fatalf("expected title %q, got %#v", task.Title, rendered)
	}
	if rendered.StepMode != CapabilityRendered {
		t.Fatalf("expected rendered step mode, got %#v", rendered)
	}
	if len(rendered.Steps) != 0 {
		t.Fatalf("expected default beads rendering to inline steps into body, got %#v", rendered.Steps)
	}

	wantBody := []string{
		"## Why",
		"Preserve old workers during rollout.",
		"## Rollback",
		"Restore the previous schema snapshot.",
		"## Dependencies",
		"- api.schema",
		"- api.runtime",
		"## Acceptance",
		"- cmd:go test ./...",
		"## Evidence",
		"- table: node.evidence.1",
		"## Provenance",
		"- source: PLAN.md:10-20",
		"- source_hash: hash-123",
		"- node_ref: PLAN.md|heading|abc#1",
		"- anchor: PLAN.md#L10",
		"## Steps",
		"- [ ] Write additive migration",
		"- [x] Verify rollback",
	}
	if !reflect.DeepEqual(rendered.Body, wantBody) {
		t.Fatalf("unexpected default body\nwant=%#v\ngot=%#v", wantBody, rendered.Body)
	}
}

func TestRenderTaskCompactProfile(t *testing.T) {
	task := fixtureRenderedTask()

	rendered, err := RenderTask(task, NewBeadsAdapter().Capabilities(), RenderProfileCompact)
	if err != nil {
		t.Fatalf("render compact profile: %v", err)
	}

	wantBody := []string{
		"why: Preserve old workers during rollout.",
		"rollback: Restore the previous schema snapshot.",
		"deps: api.schema, api.runtime",
		"accept: cmd:go test ./...",
		"evidence: table:node.evidence.1",
		"provenance: PLAN.md:10-20",
		"steps",
		"- [ ] Write additive migration",
		"- [x] Verify rollback",
	}
	if !reflect.DeepEqual(rendered.Body, wantBody) {
		t.Fatalf("unexpected compact body\nwant=%#v\ngot=%#v", wantBody, rendered.Body)
	}
}

func TestRenderTaskAgenticProfileUsesNativeStepsWhenAvailable(t *testing.T) {
	task := fixtureRenderedTask()
	caps := NewBeadsAdapter().Capabilities()
	caps.Steps = CapabilityNative

	rendered, err := RenderTask(task, caps, RenderProfileAgentic)
	if err != nil {
		t.Fatalf("render agentic profile: %v", err)
	}

	if rendered.StepMode != CapabilityNative {
		t.Fatalf("expected native step mode, got %#v", rendered)
	}
	if len(rendered.Steps) != 2 {
		t.Fatalf("expected native steps, got %#v", rendered.Steps)
	}
	for _, line := range rendered.Body {
		if strings.Contains(line, "## Steps") {
			t.Fatalf("did not expect rendered step block when native step support exists, got %#v", rendered.Body)
		}
	}
	if !strings.Contains(strings.Join(rendered.Body, "\n"), "## Task") {
		t.Fatalf("expected agentic header, got %#v", rendered.Body)
	}
}

func TestRenderTaskRejectsUnsupportedBody(t *testing.T) {
	task := fixtureRenderedTask()
	caps := NewBeadsAdapter().Capabilities()
	caps.Body = TextUnsupported

	_, err := RenderTask(task, caps, RenderProfileDefault)
	if err == nil || !strings.Contains(err.Error(), "does not support body rendering") {
		t.Fatalf("expected unsupported body error, got %v", err)
	}
}

func TestRenderTaskPlainTextBodyModeRemovesMarkdownFormatting(t *testing.T) {
	task := fixtureRenderedTask()
	caps := NewBeadsAdapter().Capabilities()
	caps.Body = TextPlain

	rendered, err := RenderTask(task, caps, RenderProfileDefault)
	if err != nil {
		t.Fatalf("render plain-text body profile: %v", err)
	}

	if strings.Contains(strings.Join(rendered.Body, "\n"), "## ") {
		t.Fatalf("expected plain-text body mode to remove markdown headings, got %#v", rendered.Body)
	}
	if strings.Contains(strings.Join(rendered.Body, "\n"), "- [ ] ") || strings.Contains(strings.Join(rendered.Body, "\n"), "- [x] ") {
		t.Fatalf("expected plain-text body mode to remove markdown checklists, got %#v", rendered.Body)
	}
	if !reflect.DeepEqual(rendered.Body[:4], []string{
		"Why:",
		"Preserve old workers during rollout.",
		"Rollback:",
		"Restore the previous schema snapshot.",
	}) {
		t.Fatalf("unexpected plain-text conversion prefix: %#v", rendered.Body)
	}
}

func TestRenderTaskRejectsUnsupportedSteps(t *testing.T) {
	task := fixtureRenderedTask()
	caps := NewBeadsAdapter().Capabilities()
	caps.Steps = CapabilityUnsupported

	_, err := RenderTask(task, caps, RenderProfileDefault)
	if err == nil || !strings.Contains(err.Error(), "does not support rendered or native steps") {
		t.Fatalf("expected unsupported steps error, got %v", err)
	}
}

func fixtureRenderedTask() TaskProjection {
	return TaskProjection{
		ID:      "api.migrate",
		Title:   "Add migration",
		Horizon: "now",
		Anchor:  "PLAN.md#L10",
		Provenance: TaskProvenance{
			NodeRef:    "PLAN.md|heading|abc#1",
			Path:       "PLAN.md",
			StartLine:  10,
			EndLine:    20,
			SourceHash: "hash-123",
			CompileID:  "compile-456",
		},
		Dependencies: []string{"api.schema", "api.runtime"},
		Acceptance:   []string{"cmd:go test ./..."},
		Steps: []TaskProjectionStep{
			{Title: "Write additive migration"},
			{Title: "Verify rollback", Checked: true},
		},
		Evidence: []TaskProjectionEvidence{
			{NodeRef: "node.evidence.1", Kind: "table"},
		},
		Sections: []TaskProjectionSection{
			{Key: "why", Title: "Why", Body: []string{"Preserve old workers during rollout."}},
			{Key: "rollback", Title: "Rollback", Body: []string{"Restore the previous schema snapshot."}},
		},
	}
}
