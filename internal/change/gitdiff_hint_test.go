package change

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/compile"
)

func TestParseUnifiedDiffHunksStable(t *testing.T) {
	diff := []byte(stringsJoinLines(
		"diff --git a/PLAN.md b/PLAN.md",
		"--- a/PLAN.md",
		"+++ b/PLAN.md",
		"@@ -4,0 +10,2 @@",
		"+new",
		"+lines",
		"@@ -20,3 +5,1 @@",
		"-old",
		"+new",
		"",
	))

	got := ParseUnifiedDiffHunks(diff)
	want := []GitDiffHunk{
		{Path: "PLAN.md", NewStart: 5, NewLength: 1},
		{Path: "PLAN.md", NewStart: 10, NewLength: 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected hunks: got=%#v want=%#v", got, want)
	}
}

func TestGitDiffHunksAreAdvisoryOnly(t *testing.T) {
	beforePlan := stringsJoinLines(
		"- [ ] Task A",
		"  @id task.a",
		"  @horizon now",
		"- [ ] Task B",
		"  @id task.b",
		"  @horizon next",
	)
	afterPlan := stringsJoinLines(
		"- [ ] Task A changed title",
		"  @id task.a",
		"  @horizon now",
		"- [ ] Task B",
		"  @id task.b",
		"  @horizon now",
	)

	before, err := compile.CompilePlan(filepath.ToSlash("before.md"), []byte(beforePlan), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile before: %v", err)
	}
	after, err := compile.CompilePlan(filepath.ToSlash("after.md"), []byte(afterPlan), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile after: %v", err)
	}
	expected := SemanticDiff(before, after)
	if len(expected) == 0 {
		t.Fatalf("expected semantic changes")
	}

	_, err = LoadPlanGitDiffHints("PLAN.md", func(args ...string) ([]byte, error) {
		return nil, errors.New("git unavailable")
	})
	if err == nil {
		t.Fatalf("expected git hint loader error when git is unavailable")
	}

	got := SemanticDiff(before, after)
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("semantic diff changed due to advisory hints failure: got=%#v want=%#v", got, expected)
	}
}

func stringsJoinLines(lines ...string) string {
	return strings.Join(lines, "\n")
}
