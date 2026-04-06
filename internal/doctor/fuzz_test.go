package doctor

import (
	"reflect"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/compile"
)

func FuzzDoctorRunDeterminism(f *testing.F) {
	f.Add([]byte("- [ ] Task A\n  @id task.a\n  @horizon now\n  @accept cmd:go test ./...\n"), "exec")
	f.Add([]byte("- [ ] Task A\n  @id task.a\n  @deps task.b\n"), "loose")

	f.Fuzz(func(t *testing.T, content []byte, profile string) {
		compiledA, errA := compile.CompilePlan("PLAN.md", content, compile.NewParser(nil))
		compiledB, errB := compile.CompilePlan("PLAN.md", content, compile.NewParser(nil))
		if (errA != nil) != (errB != nil) {
			t.Fatalf("nondeterministic compile error state: %v vs %v", errA, errB)
		}
		if errA != nil {
			if errA.Error() != errB.Error() {
				t.Fatalf("nondeterministic compile error text: %q vs %q", errA.Error(), errB.Error())
			}
			return
		}

		resultA, errA := Run(compiledA, profile)
		resultB, errB := Run(compiledB, profile)
		if (errA != nil) != (errB != nil) {
			t.Fatalf("nondeterministic doctor error state: %v vs %v", errA, errB)
		}
		if errA != nil {
			if errA.Error() != errB.Error() {
				t.Fatalf("nondeterministic doctor error text: %q vs %q", errA.Error(), errB.Error())
			}
			return
		}
		if !reflect.DeepEqual(resultA, resultB) {
			t.Fatalf("nondeterministic doctor result")
		}
	})
}

func FuzzQueryTasksDeterminism(f *testing.F) {
	f.Add([]byte("- [ ] Task A\n  @id task.a\n  @deps task.b, task.c\n"), "now")
	f.Add([]byte("- [ ] Task A\n  @id task.a\n"), "")

	f.Fuzz(func(t *testing.T, content []byte, horizon string) {
		compiled, err := compile.CompilePlan("PLAN.md", content, compile.NewParser(nil))
		if err != nil {
			return
		}

		tasksA, errA := QueryTasks(compiled, horizon)
		tasksB, errB := QueryTasks(compiled, horizon)
		if (errA != nil) != (errB != nil) {
			t.Fatalf("nondeterministic query error state: %v vs %v", errA, errB)
		}
		if errA != nil {
			if errA.Error() != errB.Error() {
				t.Fatalf("nondeterministic query error text: %q vs %q", errA.Error(), errB.Error())
			}
			return
		}
		if !reflect.DeepEqual(tasksA, tasksB) {
			t.Fatalf("nondeterministic query task result")
		}
		for i := 1; i < len(tasksA); i++ {
			if tasksA[i-1].ID > tasksA[i].ID {
				t.Fatalf("tasks not sorted by id: %#v then %#v", tasksA[i-1], tasksA[i])
			}
		}
		for _, task := range tasksA {
			for i := 1; i < len(task.Deps); i++ {
				if task.Deps[i-1] > task.Deps[i] {
					t.Fatalf("deps not sorted: %#v", task.Deps)
				}
			}
		}
	})
}
