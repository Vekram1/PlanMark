package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/compile"
)

func TestExplainReportsBlockers(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task now",
		"  @id fixture.task.now",
		"  @horizon now",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	compiled, err := compile.CompilePlan(planPath, []byte(planBody), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	explained, err := Explain(compiled, "fixture.task.now")
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	if explained.Runnable {
		t.Fatalf("expected task to be blocked, got runnable")
	}
	if len(explained.Blockers) == 0 {
		t.Fatalf("expected at least one blocker")
	}
	foundMissingAccept := false
	for _, blocker := range explained.Blockers {
		if blocker.Code == "MISSING_ACCEPT" {
			foundMissingAccept = true
			if !strings.Contains(blocker.Suggestion, "@accept") {
				t.Fatalf("expected @accept suggestion, got %q", blocker.Suggestion)
			}
		}
	}
	if !foundMissingAccept {
		t.Fatalf("expected MISSING_ACCEPT blocker, got %#v", explained.Blockers)
	}
	if len(explained.SuggestedMetadata) == 0 || !strings.Contains(explained.SuggestedMetadata[0], "@accept") {
		t.Fatalf("expected suggested metadata to include @accept, got %#v", explained.SuggestedMetadata)
	}
}

func TestOpenReturnsExactSlice(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"# Overview",
		"- [ ] Task now",
		"  @id fixture.task.now",
		"  @horizon now",
		"  @accept cmd:go test ./...",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	compiled, err := compile.CompilePlan(planPath, []byte(planBody), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	taskResult, err := Open(compiled, "fixture.task.now")
	if err != nil {
		t.Fatalf("open by task id: %v", err)
	}
	if taskResult.TaskID != "fixture.task.now" {
		t.Fatalf("expected task id fixture.task.now, got %q", taskResult.TaskID)
	}
	if !strings.Contains(taskResult.SliceText, "- [ ] Task now") {
		t.Fatalf("expected task slice text, got %q", taskResult.SliceText)
	}

	var headingRef string
	for _, node := range compiled.Source.Nodes {
		if node.Kind == "heading" && strings.Contains(node.Text, "Overview") {
			headingRef = node.NodeRef
			break
		}
	}
	if headingRef == "" {
		t.Fatalf("expected to locate heading node_ref")
	}

	headingResult, err := Open(compiled, headingRef)
	if err != nil {
		t.Fatalf("open by node_ref: %v", err)
	}
	if headingResult.NodeRef != headingRef {
		t.Fatalf("expected node_ref %q, got %q", headingRef, headingResult.NodeRef)
	}
	if headingResult.TaskID != "" {
		t.Fatalf("expected empty task id for non-task slice, got %q", headingResult.TaskID)
	}
	if strings.TrimSpace(headingResult.SliceText) != "# Overview" {
		t.Fatalf("expected exact heading slice, got %q", headingResult.SliceText)
	}
}
