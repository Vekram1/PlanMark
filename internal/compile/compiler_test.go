package compile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceRanges(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "testdata", "plans", "parser-order.md")
	fixture, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	compiled, err := CompileNodes(fixturePath, fixture, NewParser(nil))
	if err != nil {
		t.Fatalf("compile nodes: %v", err)
	}

	if len(compiled) == 0 {
		t.Fatalf("expected compiled nodes")
	}

	for _, n := range compiled {
		if n.Slice.StartLine <= 0 || n.Slice.EndLine < n.Slice.StartLine {
			t.Fatalf("invalid source range for node %q: %+v", n.Node.Text, n.Slice)
		}
		if n.Slice.StartLine != n.Node.Line || n.Slice.EndLine != n.Node.Line {
			t.Fatalf("expected line-precise range for node %q: node_line=%d slice=%d-%d", n.Node.Text, n.Node.Line, n.Slice.StartLine, n.Slice.EndLine)
		}
		if strings.TrimSpace(n.Slice.Text) == "" {
			t.Fatalf("expected non-empty slice text for node %q", n.Node.Text)
		}
	}
}

func TestSliceHashStability(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "testdata", "plans", "parser-order.md")
	fixture, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	compiledLF, err := CompileNodes(fixturePath, fixture, NewParser(nil))
	if err != nil {
		t.Fatalf("compile LF fixture: %v", err)
	}

	crlf := strings.ReplaceAll(string(fixture), "\n", "\r\n")
	compiledCRLF, err := CompileNodes(fixturePath, []byte(crlf), NewParser(nil))
	if err != nil {
		t.Fatalf("compile CRLF fixture: %v", err)
	}

	if len(compiledLF) != len(compiledCRLF) {
		t.Fatalf("node count mismatch LF=%d CRLF=%d", len(compiledLF), len(compiledCRLF))
	}

	for i := range compiledLF {
		if compiledLF[i].Slice.Hash != compiledCRLF[i].Slice.Hash {
			t.Fatalf("hash mismatch at index %d (%q): LF=%s CRLF=%s", i, compiledLF[i].Node.Text, compiledLF[i].Slice.Hash, compiledCRLF[i].Slice.Hash)
		}
	}
}
