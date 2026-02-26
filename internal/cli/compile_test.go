package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompileWithPlanFlagWritesOutput(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	outPath := filepath.Join(tmp, "out", "plan.json")

	planBody := "- [ ] Task A\n  @id fixture.task.a\n"
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"compile", "--plan", planPath, "--out", outPath}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected output file at %q: %v", outPath, err)
	}
	if !strings.Contains(out.String(), "\"tasks\"") {
		t.Fatalf("expected JSON payload on stdout, got: %q", out.String())
	}
}

func TestCompileRejectsPlanProvidedTwice(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	if err := os.WriteFile(planPath, []byte("- [ ] Task A\n"), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"compile", "--plan", planPath, planPath}, &out, &errOut)
	if exit != 2 {
		t.Fatalf("expected exit 2, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(errOut.String(), "plan path provided both positionally and via --plan") {
		t.Fatalf("unexpected stderr: %q", errOut.String())
	}
}

func TestCompilePositionalPlanThenOutFlag(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	outPath := filepath.Join(tmp, "out", "plan.json")
	if err := os.WriteFile(planPath, []byte("- [ ] Task A\n  @id fixture.task.a\n"), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"compile", planPath, "--out", outPath}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected output file at %q: %v", outPath, err)
	}
}

func TestCompileOutFlagBeforePositionalPlan(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	outPath := filepath.Join(tmp, "out", "plan.json")
	if err := os.WriteFile(planPath, []byte("- [ ] Task A\n  @id fixture.task.a\n"), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"compile", "--out", outPath, planPath}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected output file at %q: %v", outPath, err)
	}
}

func TestCompileWritesManifestWhenStateDirProvided(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	stateDir := filepath.Join(tmp, ".planmark-state")
	if err := os.WriteFile(planPath, []byte("- [ ] Task A\n  @id fixture.task.a\n"), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"compile", "--plan", planPath, "--state-dir", stateDir}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}

	manifestPath := filepath.Join(stateDir, "build", "compile-manifest.json")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("expected compile manifest at %q: %v", manifestPath, err)
	}
}

func TestCompileGitDiffHintsAreAdvisoryOnly(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	if err := os.WriteFile(planPath, []byte("- [ ] Task A\n  @id fixture.task.a\n"), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"compile", "--plan", planPath, "--git-diff-hints"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(out.String(), "\"tasks\"") {
		t.Fatalf("expected compile output json, got %q", out.String())
	}
}
