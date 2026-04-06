package ir_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/compile"
)

func FuzzCompileJSONDeterminism(f *testing.F) {
	f.Add([]byte("- [ ] Task A\n  @id task.a\n  @horizon now\n  @accept cmd:go test ./...\n"))
	f.Add([]byte("- [ ] Task B\n  @id task.b\n"))

	f.Fuzz(func(t *testing.T, content []byte) {
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

		jsonA, err := json.MarshalIndent(compiledA, "", "  ")
		if err != nil {
			t.Fatalf("marshal compile json A: %v", err)
		}
		jsonB, err := json.MarshalIndent(compiledB, "", "  ")
		if err != nil {
			t.Fatalf("marshal compile json B: %v", err)
		}
		jsonA = append(jsonA, '\n')
		jsonB = append(jsonB, '\n')
		if !bytes.Equal(jsonA, jsonB) {
			t.Fatalf("nondeterministic compiled json bytes")
		}
	})
}
