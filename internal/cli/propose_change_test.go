package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProposeChangeJSONStableForMissingAccept(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task now missing accept",
		"  @id fixture.task.now",
		"  @horizon now",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	run := func() map[string]any {
		var out bytes.Buffer
		var errOut bytes.Buffer
		exit := Run([]string{"propose-change", "--plan", planPath, "--format", "json", "fixture.task.now"}, &out, &errOut)
		if exit != 0 {
			t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
		}
		var payload map[string]any
		if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
			t.Fatalf("decode output: %v output=%q", err, out.String())
		}
		return payload
	}

	outA := run()
	outB := run()
	rawA, _ := json.Marshal(outA)
	rawB, _ := json.Marshal(outB)
	if string(rawA) != string(rawB) {
		t.Fatalf("expected deterministic propose-change output, got A=%s B=%s", string(rawA), string(rawB))
	}

	data, ok := outA["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object data payload")
	}
	delta, ok := data["delta"].(map[string]any)
	if !ok {
		t.Fatalf("expected delta object")
	}
	if delta["schema_version"] != "v0.1" {
		t.Fatalf("expected delta schema_version v0.1, got %v", delta["schema_version"])
	}
	ops, ok := delta["operations"].([]any)
	if !ok || len(ops) != 1 {
		t.Fatalf("expected one proposed operation, got %v", delta["operations"])
	}
	op, _ := ops[0].(map[string]any)
	if op["kind"] != "metadata_upsert" {
		t.Fatalf("expected metadata_upsert op, got %v", op["kind"])
	}
}

func TestProposeChangeRejectsMissingTaskID(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"propose-change", "--plan", "PLAN.md"}, &out, &errOut)
	if exit != 2 {
		t.Fatalf("expected usage exit 2, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(errOut.String(), "usage: plan propose-change") {
		t.Fatalf("expected usage text, got %q", errOut.String())
	}
}

func TestRunDispatchesProposeChange(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task now missing accept",
		"  @id fixture.task.now",
		"  @horizon now",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"propose-change", "--plan", planPath, "--format", "text", "fixture.task.now"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(out.String(), "operations_count: 1") {
		t.Fatalf("expected an operation proposal in output, got %q", out.String())
	}
}

func TestProposeChangeSuggestedDiffUsesResolvedPlanPath(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "custom-plan.md")
	planBody := strings.Join([]string{
		"- [ ] Task now missing accept",
		"  @id fixture.task.now",
		"  @horizon now",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"propose-change", "--plan", planPath, "--format", "json", "fixture.task.now"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}

	var payload struct {
		Data struct {
			SuggestedDiff []string `json:"suggested_diff"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode output: %v output=%q", err, out.String())
	}
	if len(payload.Data.SuggestedDiff) != 1 {
		t.Fatalf("expected one suggested diff line, got %v", payload.Data.SuggestedDiff)
	}
	if !strings.Contains(payload.Data.SuggestedDiff[0], filepath.ToSlash(planPath)+":") {
		t.Fatalf("expected suggested diff to include resolved plan path, got %q", payload.Data.SuggestedDiff[0])
	}
}
