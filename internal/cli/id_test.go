package cli

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestIDScaffoldDeterministic(t *testing.T) {
	if got := scaffoldID("  Add Plan ID helper!! "); got != "add.plan.id.helper" {
		t.Fatalf("unexpected scaffold id: %q", got)
	}
	if got := scaffoldID("A---B___C"); got != "a.b.c" {
		t.Fatalf("unexpected normalized separators: %q", got)
	}
}

func TestIDCommandTextAndJSON(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"id", "Task Title 123"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected success, got exit=%d stderr=%q", exit, errOut.String())
	}
	if out.String() != "task.title.123\n" {
		t.Fatalf("unexpected text output: %q", out.String())
	}

	out.Reset()
	errOut.Reset()
	exit = Run([]string{"id", "--format", "json", "Task Title 123"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected success, got exit=%d stderr=%q", exit, errOut.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json output: %v", err)
	}
	if payload["command"] != "id" {
		t.Fatalf("expected command id, got %v", payload["command"])
	}
	data := payload["data"].(map[string]any)
	if data["id"] != "task.title.123" {
		t.Fatalf("unexpected id value: %v", data["id"])
	}
}
