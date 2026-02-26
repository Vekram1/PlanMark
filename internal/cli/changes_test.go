package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/change"
	"github.com/vikramoddiraju/planmark/internal/compile"
)

func TestChangesCLIJSONOutputStable(t *testing.T) {
	tmp := t.TempDir()
	stateDir := filepath.Join(tmp, ".planmark")
	planPath := filepath.Join(tmp, "PLAN.md")

	planA := strings.Join([]string{
		"- [ ] Task now",
		"  @id fixture.task.now",
		"  @horizon now",
		"  @accept cmd:go test ./...",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planA), 0o644); err != nil {
		t.Fatalf("write plan A: %v", err)
	}

	seedOutput := runChangesJSON(t, []string{"changes", "--plan", planPath, "--state-dir", stateDir, "--format", "json"})
	seedCompileID := jsonPathString(t, seedOutput, "data", "current_compile_id")
	if seedCompileID == "" {
		t.Fatalf("expected non-empty current_compile_id")
	}

	planB := strings.Replace(planA, "Task now", "Task now renamed", 1)
	if err := os.WriteFile(planPath, []byte(planB), 0o644); err != nil {
		t.Fatalf("write plan B: %v", err)
	}

	args := []string{"changes", "--plan", planPath, "--state-dir", stateDir, "--since", seedCompileID, "--format", "json"}
	outA := runChangesJSON(t, args)
	outB := runChangesJSON(t, args)

	if !reflect.DeepEqual(outA, outB) {
		t.Fatalf("expected deterministic JSON output for identical baseline and plan")
	}

	changes, ok := jsonPath(outA, "data", "changes").([]interface{})
	if !ok {
		t.Fatalf("expected changes array in output")
	}
	if len(changes) == 0 {
		t.Fatalf("expected at least one semantic change after plan edit")
	}
}

func TestChangesCLIRequiresMatchingSinceCompileID(t *testing.T) {
	tmp := t.TempDir()
	stateDir := filepath.Join(tmp, ".planmark")
	planPath := filepath.Join(tmp, "PLAN.md")
	plan := strings.Join([]string{
		"- [ ] Task now",
		"  @id fixture.task.now",
		"  @horizon now",
		"  @accept cmd:go test ./...",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(plan), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	_ = runChangesJSON(t, []string{"changes", "--plan", planPath, "--state-dir", stateDir, "--format", "json"})

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"changes", "--plan", planPath, "--state-dir", stateDir, "--since", "not-a-real-compile-id", "--format", "json"}, &out, &errOut)
	if exit != 1 {
		t.Fatalf("expected validation exit code 1, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(errOut.String(), "does not match available baseline") {
		t.Fatalf("expected baseline mismatch error, got %q", errOut.String())
	}
}

func TestChangesCLIFailsWhenSinceBaselineSnapshotMissing(t *testing.T) {
	tmp := t.TempDir()
	stateDir := filepath.Join(tmp, ".planmark")
	planPath := filepath.Join(tmp, "PLAN.md")
	plan := strings.Join([]string{
		"- [ ] Task now",
		"  @id fixture.task.now",
		"  @horizon now",
		"  @accept cmd:go test ./...",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(plan), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	seed := runChangesJSON(t, []string{"changes", "--plan", planPath, "--state-dir", stateDir, "--format", "json"})
	seedCompileID := jsonPathString(t, seed, "data", "current_compile_id")

	if err := os.Remove(filepath.Join(stateDir, "build", "plan-latest.json")); err != nil {
		t.Fatalf("remove baseline snapshot: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"changes", "--plan", planPath, "--state-dir", stateDir, "--since", seedCompileID, "--format", "json"}, &out, &errOut)
	if exit != 1 {
		t.Fatalf("expected validation exit code 1, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(errOut.String(), "baseline plan snapshot is missing") {
		t.Fatalf("expected missing baseline snapshot error, got %q", errOut.String())
	}
}

func TestChangesSinceGitStillUsesCanonicalDiff(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark")

	runGit(t, tmp, "init")
	runGit(t, tmp, "config", "user.email", "planmark-tests@example.com")
	runGit(t, tmp, "config", "user.name", "PlanMark Tests")

	basePlan := strings.Join([]string{
		"- [ ] Task now",
		"  @id fixture.task.now",
		"  @horizon now",
		"  @accept cmd:go test ./...",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(basePlan), 0o644); err != nil {
		t.Fatalf("write base plan: %v", err)
	}
	runGit(t, tmp, "add", "PLAN.md")
	runGit(t, tmp, "commit", "-m", "base plan")

	currentPlan := strings.Replace(basePlan, "Task now", "Task now changed", 1)
	if err := os.WriteFile(planPath, []byte(currentPlan), 0o644); err != nil {
		t.Fatalf("write current plan: %v", err)
	}

	baselineContent, err := change.LoadPlanContentAtGitRef(planPath, "HEAD", nil)
	if err != nil {
		t.Fatalf("load baseline content from git: %v", err)
	}
	baselineIR, err := compile.CompilePlan(planPath, baselineContent, compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile baseline plan: %v", err)
	}
	currentIR, err := compile.CompilePlan(planPath, []byte(currentPlan), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile current plan: %v", err)
	}
	expected := change.SemanticDiff(baselineIR, currentIR)

	out := runChangesJSON(t, []string{"changes", "--plan", planPath, "--state-dir", stateDir, "--since", "HEAD", "--format", "json"})
	if got := jsonPathString(t, out, "data", "since_git_ref"); got != "HEAD" {
		t.Fatalf("expected since_git_ref=HEAD, got %q", got)
	}

	rawChanges, ok := jsonPath(out, "data", "changes").([]interface{})
	if !ok {
		t.Fatalf("expected changes array in output")
	}
	got := make([]change.TaskChange, 0, len(rawChanges))
	for _, item := range rawChanges {
		raw, err := json.Marshal(item)
		if err != nil {
			t.Fatalf("marshal change item: %v", err)
		}
		var entry change.TaskChange
		if err := json.Unmarshal(raw, &entry); err != nil {
			t.Fatalf("unmarshal change item: %v", err)
		}
		got = append(got, entry)
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("expected canonical semantic diff unchanged in --since git mode\nexpected=%#v\ngot=%#v", expected, got)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, fmt.Sprintf("%s", out))
	}
}

func runChangesJSON(t *testing.T, args []string) map[string]interface{} {
	t.Helper()

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run(args, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode changes json: %v", err)
	}
	if payload["command"] != "changes" {
		t.Fatalf("expected command changes, got %v", payload["command"])
	}
	if payload["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", payload["status"])
	}
	return payload
}

func jsonPathString(t *testing.T, m map[string]interface{}, path ...string) string {
	t.Helper()
	value := jsonPath(m, path...)
	result, ok := value.(string)
	if !ok {
		t.Fatalf("expected string at path %v, got %T", path, value)
	}
	return result
}

func jsonPath(m map[string]interface{}, path ...string) interface{} {
	current := interface{}(m)
	for _, key := range path {
		next, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}
		current = next[key]
	}
	return current
}
