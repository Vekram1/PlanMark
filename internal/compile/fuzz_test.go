package compile

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func FuzzMetadataParse(f *testing.F) {
	for _, seed := range fuzzSeeds(f) {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, content []byte) {
		first, errA := ParseMetadata(content)
		second, errB := ParseMetadata(content)

		if (errA != nil) != (errB != nil) {
			t.Fatalf("nondeterministic metadata parse error state: errA=%v errB=%v", errA, errB)
		}
		if errA != nil {
			if errA.Error() != errB.Error() {
				t.Fatalf("nondeterministic metadata parse error text: %q vs %q", errA.Error(), errB.Error())
			}
			return
		}
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("nondeterministic metadata parse result")
		}
	})
}

func FuzzMetadataAttach(f *testing.F) {
	for _, seed := range fuzzSeeds(f) {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, content []byte) {
		parser := NewParser(nil)
		nodes, errA := parser.Parse("fuzz.md", content)
		nodesAgain, errB := parser.Parse("fuzz.md", content)
		if (errA != nil) != (errB != nil) {
			t.Fatalf("nondeterministic node parse error state: errA=%v errB=%v", errA, errB)
		}
		if errA != nil {
			if errA.Error() != errB.Error() {
				t.Fatalf("nondeterministic node parse error text: %q vs %q", errA.Error(), errB.Error())
			}
			return
		}
		if !reflect.DeepEqual(nodes, nodesAgain) {
			t.Fatalf("nondeterministic node parse result")
		}

		parsed, err := ParseMetadata(content)
		if err != nil {
			return
		}

		attachedA := AttachMetadataToNodes(nodes, parsed)
		attachedB := AttachMetadataToNodes(nodesAgain, parsed)
		if !reflect.DeepEqual(attachedA, attachedB) {
			t.Fatalf("nondeterministic metadata attachment")
		}
	})
}

func FuzzSpanRecovery(f *testing.F) {
	for _, seed := range fuzzSeeds(f) {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, content []byte) {
		compiledA, errA := CompileNodes("fuzz.md", content, NewParser(nil))
		compiledB, errB := CompileNodes("fuzz.md", content, NewParser(nil))

		if (errA != nil) != (errB != nil) {
			t.Fatalf("nondeterministic CompileNodes error state: errA=%v errB=%v", errA, errB)
		}
		if errA != nil {
			if errA.Error() != errB.Error() {
				t.Fatalf("nondeterministic CompileNodes error text: %q vs %q", errA.Error(), errB.Error())
			}
			return
		}
		if !reflect.DeepEqual(compiledA, compiledB) {
			t.Fatalf("nondeterministic CompileNodes result")
		}

		totalLines := len(normalizedLines(string(content)))
		for _, node := range compiledA {
			if node.Slice.StartLine <= 0 || node.Slice.EndLine < node.Slice.StartLine {
				t.Fatalf("invalid recovered slice range: %+v", node.Slice)
			}
			if node.Slice.EndLine > totalLines {
				t.Fatalf("slice range exceeds content lines: range=%d-%d total=%d", node.Slice.StartLine, node.Slice.EndLine, totalLines)
			}
		}
	})
}

func FuzzCompileDoesNotPanic(f *testing.F) {
	for _, seed := range fuzzSeeds(f) {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, content []byte) {
		planA, errA := CompilePlan("fuzz.md", content, NewParser(nil))
		planB, errB := CompilePlan("fuzz.md", content, NewParser(nil))
		if (errA != nil) != (errB != nil) {
			t.Fatalf("nondeterministic CompilePlan error state: errA=%v errB=%v", errA, errB)
		}
		if errA != nil {
			if errA.Error() != errB.Error() {
				t.Fatalf("nondeterministic CompilePlan error text: %q vs %q", errA.Error(), errB.Error())
			}
			return
		}
		if !reflect.DeepEqual(planA, planB) {
			t.Fatalf("nondeterministic CompilePlan result")
		}
	})
}

func fuzzSeeds(tb testing.TB) [][]byte {
	tb.Helper()
	seedDirs := []string{
		filepath.Join("..", "..", "testdata", "malformed"),
		filepath.Join("..", "..", "testdata", "plans"),
	}
	seeds := make([][]byte, 0)
	for _, dir := range seedDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				continue
			}
			seeds = append(seeds, raw)
		}
	}
	if len(seeds) == 0 {
		seeds = append(seeds, []byte("- [ ] fallback\n@id fallback.id\n"))
	}
	return seeds
}
