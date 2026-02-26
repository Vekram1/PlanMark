package change

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/compile"
)

func TestSemanticDiffClassificationStable(t *testing.T) {
	beforePlan := strings.Join([]string{
		"- [ ] Keep same",
		"  @id task.same",
		"  @horizon now",
		"  @accept cmd:go test ./...",
		"- [ ] Deps change",
		"  @id task.deps",
		"  @horizon now",
		"  @deps dep.one",
		"  @accept cmd:go test ./...",
		"- [ ] Accept change",
		"  @id task.accept",
		"  @horizon now",
		"  @accept cmd:go test ./...",
		"- [ ] Horizon change",
		"  @id task.horizon",
		"  @horizon next",
		"- [ ] Metadata change old",
		"  @id task.meta",
		"  @horizon now",
		"  @accept cmd:go test ./...",
		"- [ ] Move source",
		"  @id task.oldid",
		"  @horizon now",
		"  @accept cmd:go test ./...",
		"- [ ] Delete me",
		"  @id task.deleted",
		"  @horizon now",
		"  @accept cmd:go test ./...",
	}, "\n")

	afterPlan := strings.Join([]string{
		"- [ ] Keep same",
		"  @id task.same",
		"  @horizon now",
		"  @accept cmd:go test ./...",
		"- [ ] Deps change",
		"  @id task.deps",
		"  @horizon now",
		"  @deps dep.one,dep.two",
		"  @accept cmd:go test ./...",
		"- [ ] Accept change",
		"  @id task.accept",
		"  @horizon now",
		"  @accept cmd:go test ./... -run TestSomething",
		"- [ ] Horizon change",
		"  @id task.horizon",
		"  @horizon now",
		"  @accept cmd:go test ./...",
		"- [ ] Metadata change new title",
		"  @id task.meta",
		"  @horizon now",
		"  @accept cmd:go test ./...",
		"- [ ] Move target",
		"  @id task.newid",
		"  @supersedes task.oldid",
		"  @horizon now",
		"  @accept cmd:go test ./...",
		"- [ ] Added task",
		"  @id task.added",
		"  @horizon next",
	}, "\n")

	before, err := compile.CompilePlan(filepath.ToSlash("before.md"), []byte(beforePlan), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile before plan: %v", err)
	}
	after, err := compile.CompilePlan(filepath.ToSlash("after.md"), []byte(afterPlan), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile after plan: %v", err)
	}

	first := SemanticDiff(before, after)
	second := SemanticDiff(before, after)
	if len(first) != len(second) {
		t.Fatalf("non-deterministic change count: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("non-deterministic change at index %d: %#v vs %#v", i, first[i], second[i])
		}
	}

	expect := map[string]string{
		"task.added":   ClassAdded,
		"task.deleted": ClassDeleted,
		"task.deps":    ClassDepsChange,
		"task.accept":  ClassAcceptChange,
		"task.horizon": ClassHorizonChange,
		"task.meta":    ClassMetadataChange,
	}
	for _, change := range first {
		if change.Class == ClassMoved {
			if change.OldID != "task.oldid" || change.NewID != "task.newid" {
				t.Fatalf("unexpected moved mapping: %#v", change)
			}
			continue
		}
		if expectedClass, ok := expect[change.TaskID]; ok {
			if expectedClass != change.Class {
				t.Fatalf("unexpected class for %s: got %s want %s", change.TaskID, change.Class, expectedClass)
			}
			delete(expect, change.TaskID)
		}
	}
	if len(expect) != 0 {
		t.Fatalf("missing expected changes: %#v", expect)
	}
}
