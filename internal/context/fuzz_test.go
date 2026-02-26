package context

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/compile"
)

func FuzzPinPathSafety(f *testing.F) {
	for _, seed := range []string{
		"internal/compile/parser.go",
		"../escape.txt",
		"/abs/path.txt",
		"linked.txt",
		".",
		"",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, target string) {
		repoRoot := t.TempDir()
		outsideRoot := t.TempDir()
		if err := os.MkdirAll(filepath.Join(repoRoot, "internal", "compile"), 0o755); err != nil {
			t.Fatalf("mkdir repo target dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(repoRoot, "internal", "compile", "parser.go"), []byte("package compile\n"), 0o644); err != nil {
			t.Fatalf("write repo target file: %v", err)
		}
		if err := os.WriteFile(filepath.Join(outsideRoot, "outside.txt"), []byte("outside\n"), 0o644); err != nil {
			t.Fatalf("write outside file: %v", err)
		}
		_ = os.Symlink(filepath.Join(outsideRoot, "outside.txt"), filepath.Join(repoRoot, "linked.txt"))

		normA, absA, errA := resolveRepoScopedPath(repoRoot, target)
		normB, absB, errB := resolveRepoScopedPath(repoRoot, target)
		if (errA != nil) != (errB != nil) {
			t.Fatalf("nondeterministic path safety error state: errA=%v errB=%v", errA, errB)
		}
		if errA != nil {
			if errA.Error() != errB.Error() {
				t.Fatalf("nondeterministic path safety error text: %q vs %q", errA.Error(), errB.Error())
			}
			return
		}
		if normA != normB || absA != absB {
			t.Fatalf("nondeterministic path resolution: (%q,%q) vs (%q,%q)", normA, absA, normB, absB)
		}
		if strings.HasPrefix(normA, "../") || normA == ".." || filepath.IsAbs(normA) {
			t.Fatalf("unsafe normalized path returned: %q", normA)
		}
	})
}

func FuzzAnchorParse(f *testing.F) {
	for _, seed := range []string{
		"internal/compile/parser.go",
		"internal/compile/parser.go:1",
		"internal/compile/parser.go:1-3",
		"internal/compile/parser.go:3-1",
		"",
		":",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, ref string) {
		pathA, startA, endA, errA := parsePinTarget(ref)
		pathB, startB, endB, errB := parsePinTarget(ref)
		if (errA != nil) != (errB != nil) {
			t.Fatalf("nondeterministic parse error state: errA=%v errB=%v", errA, errB)
		}
		if errA != nil {
			if errA.Error() != errB.Error() {
				t.Fatalf("nondeterministic parse error text: %q vs %q", errA.Error(), errB.Error())
			}
			return
		}
		if pathA != pathB || startA != startB || endA != endB {
			t.Fatalf("nondeterministic parse output")
		}
		if strings.TrimSpace(pathA) == "" {
			t.Fatalf("expected non-empty parsed path")
		}
		if startA < 0 {
			t.Fatalf("expected start >= 0, got %d", startA)
		}
		if endA != 0 && endA < startA {
			t.Fatalf("invalid parsed range: %d-%d", startA, endA)
		}
	})
}

func FuzzPinExtractDeterminism(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte("internal/compile/parser.go:1-2"),
		[]byte("internal/compile/parser.go"),
		[]byte("../outside.txt:1"),
		[]byte("linked.txt:1"),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, pinRef []byte) {
		repoRoot := t.TempDir()
		outsideRoot := t.TempDir()

		targetDir := filepath.Join(repoRoot, "internal", "compile")
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			t.Fatalf("mkdir target dir: %v", err)
		}
		targetBody := strings.Join([]string{
			"package compile",
			"",
			"func Parse() {}",
		}, "\n")
		if err := os.WriteFile(filepath.Join(targetDir, "parser.go"), []byte(targetBody), 0o644); err != nil {
			t.Fatalf("write target file: %v", err)
		}
		if err := os.WriteFile(filepath.Join(outsideRoot, "outside.txt"), []byte("outside\n"), 0o644); err != nil {
			t.Fatalf("write outside file: %v", err)
		}
		_ = os.Symlink(filepath.Join(outsideRoot, "outside.txt"), filepath.Join(repoRoot, "linked.txt"))

		planPath := filepath.Join(repoRoot, "PLAN.md")
		planBody := strings.Join([]string{
			"- [ ] Task now",
			"  @id fuzz.task.now",
			"  @horizon now",
			"  @accept cmd:go test ./...",
			"  @pin " + strings.TrimSpace(string(pinRef)),
		}, "\n")
		if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
			t.Fatalf("write plan: %v", err)
		}

		compiled, err := compile.CompilePlan(planPath, []byte(planBody), compile.NewParser(nil))
		if err != nil {
			t.Fatalf("compile plan: %v", err)
		}

		packetA, errA := BuildL1(compiled, "fuzz.task.now")
		packetB, errB := BuildL1(compiled, "fuzz.task.now")
		if (errA != nil) != (errB != nil) {
			t.Fatalf("nondeterministic pin extract error state: errA=%v errB=%v", errA, errB)
		}
		if errA != nil {
			if errA.Error() != errB.Error() {
				t.Fatalf("nondeterministic pin extract error text: %q vs %q", errA.Error(), errB.Error())
			}
			return
		}
		if !reflect.DeepEqual(packetA, packetB) {
			t.Fatalf("nondeterministic BuildL1 output")
		}
	})
}
