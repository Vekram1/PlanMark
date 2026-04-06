package change

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/compile"
)

func FuzzSemanticDiffDeterminism(f *testing.F) {
	f.Add([]byte("- [ ] Task A\n  @id task.a\n  @horizon now\n"), []byte("- [ ] Task A changed\n  @id task.a\n  @horizon next\n"))
	f.Add([]byte("- [ ] Task A\n  @id task.a\n"), []byte("- [ ] Task B\n  @id task.b\n"))

	f.Fuzz(func(t *testing.T, beforeContent []byte, afterContent []byte) {
		before, errA := compile.CompilePlan(filepath.ToSlash("before.md"), beforeContent, compile.NewParser(nil))
		beforeAgain, errB := compile.CompilePlan(filepath.ToSlash("before.md"), beforeContent, compile.NewParser(nil))
		if (errA != nil) != (errB != nil) {
			t.Fatalf("nondeterministic before compile error state: %v vs %v", errA, errB)
		}
		if errA != nil {
			if errA.Error() != errB.Error() {
				t.Fatalf("nondeterministic before compile error text: %q vs %q", errA.Error(), errB.Error())
			}
			return
		}

		after, errA := compile.CompilePlan(filepath.ToSlash("after.md"), afterContent, compile.NewParser(nil))
		afterAgain, errB := compile.CompilePlan(filepath.ToSlash("after.md"), afterContent, compile.NewParser(nil))
		if (errA != nil) != (errB != nil) {
			t.Fatalf("nondeterministic after compile error state: %v vs %v", errA, errB)
		}
		if errA != nil {
			if errA.Error() != errB.Error() {
				t.Fatalf("nondeterministic after compile error text: %q vs %q", errA.Error(), errB.Error())
			}
			return
		}

		diffA := SemanticDiff(before, after)
		diffB := SemanticDiff(beforeAgain, afterAgain)
		if !reflect.DeepEqual(diffA, diffB) {
			t.Fatalf("nondeterministic semantic diff")
		}
		assertSortedTaskChanges(t, diffA)
	})
}

func FuzzParseUnifiedDiffHunksDeterminism(f *testing.F) {
	f.Add([]byte("diff --git a/PLAN.md b/PLAN.md\n--- a/PLAN.md\n+++ b/PLAN.md\n@@ -1,0 +2,1 @@\n+line\n"))
	f.Add([]byte("@@ -4 +8,2 @@\n"))

	f.Fuzz(func(t *testing.T, diff []byte) {
		hunksA := ParseUnifiedDiffHunks(diff)
		hunksB := ParseUnifiedDiffHunks(diff)
		if !reflect.DeepEqual(hunksA, hunksB) {
			t.Fatalf("nondeterministic unified diff parsing")
		}
		for i := 1; i < len(hunksA); i++ {
			prev := hunksA[i-1]
			curr := hunksA[i]
			if prev.Path > curr.Path {
				t.Fatalf("hunks not sorted by path: %#v then %#v", prev, curr)
			}
			if prev.Path == curr.Path && prev.NewStart > curr.NewStart {
				t.Fatalf("hunks not sorted by start: %#v then %#v", prev, curr)
			}
			if prev.Path == curr.Path && prev.NewStart == curr.NewStart && prev.NewLength > curr.NewLength {
				t.Fatalf("hunks not sorted by length: %#v then %#v", prev, curr)
			}
		}
	})
}

func assertSortedTaskChanges(t *testing.T, changes []TaskChange) {
	t.Helper()
	for i := 1; i < len(changes); i++ {
		prev := changes[i-1]
		curr := changes[i]
		if prev.Class > curr.Class {
			t.Fatalf("changes not sorted by class: %#v then %#v", prev, curr)
		}
		if prev.Class == curr.Class && prev.TaskID > curr.TaskID {
			t.Fatalf("changes not sorted by task id: %#v then %#v", prev, curr)
		}
		if prev.Class == curr.Class && prev.TaskID == curr.TaskID && prev.OldID > curr.OldID {
			t.Fatalf("changes not sorted by old id: %#v then %#v", prev, curr)
		}
		if prev.Class == curr.Class && prev.TaskID == curr.TaskID && prev.OldID == curr.OldID && prev.NewID > curr.NewID {
			t.Fatalf("changes not sorted by new id: %#v then %#v", prev, curr)
		}
	}
}
