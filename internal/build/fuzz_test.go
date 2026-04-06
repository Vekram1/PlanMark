package build_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/build"
	"github.com/vikramoddiraju/planmark/internal/compile"
)

func FuzzTaskSemanticFingerprintDeterminism(f *testing.F) {
	f.Add([]byte("- [ ] Task A\n  @id task.a\n  @horizon now\n  @deps dep.a, dep.b\n  @accept cmd:go test ./...\n"))
	f.Add([]byte("- [ ] Task B\n  @id task.b\n"))

	f.Fuzz(func(t *testing.T, content []byte) {
		compiled, err := compile.CompilePlan("PLAN.md", content, compile.NewParser(nil))
		if err != nil {
			return
		}
		for _, task := range compiled.Semantic.Tasks {
			fpA := build.TaskSemanticFingerprint(task)
			fpB := build.TaskSemanticFingerprint(task)
			if fpA != fpB {
				t.Fatalf("nondeterministic task semantic fingerprint for %q", task.ID)
			}
		}
	})
}

func FuzzBuildCompileManifestDeterminism(f *testing.F) {
	f.Add([]byte("- [ ] Task A\n  @id task.a\n  @horizon now\n  @accept cmd:go test ./...\n"), "config-hash-a")
	f.Add([]byte("- [ ] Task B\n  @id task.b\n"), "")

	f.Fuzz(func(t *testing.T, content []byte, configHash string) {
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

		planJSONA, err := json.MarshalIndent(compiledA, "", "  ")
		if err != nil {
			t.Fatalf("marshal plan json A: %v", err)
		}
		planJSONB, err := json.MarshalIndent(compiledB, "", "  ")
		if err != nil {
			t.Fatalf("marshal plan json B: %v", err)
		}
		planJSONA = append(planJSONA, '\n')
		planJSONB = append(planJSONB, '\n')

		manifestA := build.BuildCompileManifest(compiledA, content, planJSONA, configHash)
		manifestB := build.BuildCompileManifest(compiledB, content, planJSONB, configHash)

		blobA, err := json.Marshal(manifestA)
		if err != nil {
			t.Fatalf("marshal manifest A: %v", err)
		}
		blobB, err := json.Marshal(manifestB)
		if err != nil {
			t.Fatalf("marshal manifest B: %v", err)
		}
		if !bytes.Equal(blobA, blobB) {
			t.Fatalf("nondeterministic compile manifest json")
		}
		if manifestA.CompileID == "" {
			t.Fatalf("expected compile id")
		}
		if manifestA.EffectiveConfigHash == "" {
			t.Fatalf("expected effective config hash")
		}
	})
}
