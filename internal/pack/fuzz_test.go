package pack

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSafeTaskPathSegmentAvoidsDotTraversal(t *testing.T) {
	for _, input := range []string{"", ".", "..", " .. "} {
		got := safeTaskPathSegment(input)
		if got != "task" {
			t.Fatalf("expected fallback segment for %q, got %q", input, got)
		}
	}
}

func FuzzSafeTaskPathSegment(f *testing.F) {
	for _, seed := range []string{"task.a", "", ".", "..", "../escape", "a/b"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, taskID string) {
		first := safeTaskPathSegment(taskID)
		second := safeTaskPathSegment(taskID)
		if first != second {
			t.Fatalf("nondeterministic task path segment: %q vs %q", first, second)
		}
		if first == "" || first == "." || first == ".." {
			t.Fatalf("unsafe task path segment %q for input %q", first, taskID)
		}
	})
}

func FuzzExportPackDeterminism(f *testing.F) {
	f.Add([]byte("- [ ] Task A\n  @id task.a\n  @horizon now\n  @accept cmd:echo ok\n"))
	f.Add([]byte("- [ ] Task A\n  @id ..\n  @horizon now\n"))

	f.Fuzz(func(t *testing.T, content []byte) {
		tmp := t.TempDir()
		planPath := filepath.Join(tmp, "PLAN.md")
		if err := os.WriteFile(planPath, content, 0o644); err != nil {
			t.Fatalf("write plan fixture: %v", err)
		}

		outA := filepath.Join(tmp, "out-a")
		outB := filepath.Join(tmp, "out-b")
		resultA, errA := Export(Options{PlanPath: planPath, OutPath: outA, Levels: []string{"L0"}})
		resultB, errB := Export(Options{PlanPath: planPath, OutPath: outB, Levels: []string{"L0"}})
		if (errA != nil) != (errB != nil) {
			t.Fatalf("nondeterministic export error state: %v vs %v", errA, errB)
		}
		if errA != nil {
			if errA.Error() != errB.Error() {
				t.Fatalf("nondeterministic export error text: %q vs %q", errA.Error(), errB.Error())
			}
			return
		}
		if resultA.PackID != resultB.PackID || resultA.TaskCount != resultB.TaskCount {
			t.Fatalf("nondeterministic export result metadata")
		}

		indexA, err := os.ReadFile(filepath.Join(outA, "pack", "index.json"))
		if err != nil {
			t.Fatalf("read index A: %v", err)
		}
		indexB, err := os.ReadFile(filepath.Join(outB, "pack", "index.json"))
		if err != nil {
			t.Fatalf("read index B: %v", err)
		}
		if !bytes.Equal(indexA, indexB) {
			t.Fatalf("nondeterministic pack index bytes")
		}
	})
}
