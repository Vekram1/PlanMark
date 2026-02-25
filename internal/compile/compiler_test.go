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

func TestMetadataAttachmentRules(t *testing.T) {
	src := strings.Join([]string{
		"@id unattached.pre", // unattached by design (no prior node)
		"# H1",
		"@horizon now",
		"- [ ] task one",
		"@accept cmd:go test ./...",
		"@unknown_thing opaque payload",
		"## H2",
		"@deps fixture.h1",
	}, "\n")

	nodes, err := NewParser(nil).Parse("attach.md", []byte(src))
	if err != nil {
		t.Fatalf("parse nodes: %v", err)
	}
	parsed, err := ParseMetadata([]byte(src))
	if err != nil {
		t.Fatalf("parse metadata: %v", err)
	}

	attached := AttachMetadataToNodes(nodes, parsed)
	if len(attached.Nodes) != len(nodes) {
		t.Fatalf("expected %d attached nodes, got %d", len(nodes), len(attached.Nodes))
	}
	if len(attached.Unattached) != 1 {
		t.Fatalf("expected 1 unattached metadata entry, got %d", len(attached.Unattached))
	}
	if attached.Unattached[0].Key != "id" || attached.Unattached[0].Value != "unattached.pre" {
		t.Fatalf("unexpected unattached metadata: %#v", attached.Unattached[0])
	}

	if len(attached.Nodes[0].KnownByKey["horizon"]) != 1 {
		t.Fatalf("expected horizon attached to first node, got %#v", attached.Nodes[0].KnownByKey)
	}
	if got := attached.Nodes[0].KnownByKey["horizon"][0].Value; got != "now" {
		t.Fatalf("unexpected horizon value: %q", got)
	}

	if len(attached.Nodes[1].KnownByKey["accept"]) != 1 {
		t.Fatalf("expected accept attached to second node, got %#v", attached.Nodes[1].KnownByKey)
	}
	if len(attached.Nodes[1].Opaque) != 1 || attached.Nodes[1].Opaque[0].Key != "unknown_thing" {
		t.Fatalf("expected unknown key in opaque metadata for second node, got %#v", attached.Nodes[1].Opaque)
	}

	if len(attached.Nodes[2].KnownByKey["deps"]) != 1 {
		t.Fatalf("expected deps attached to third node, got %#v", attached.Nodes[2].KnownByKey)
	}
}

func TestCompile(t *testing.T) {
	planPath := filepath.Join("..", "..", "testdata", "plans", "mixed.md")
	content, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("read plan fixture: %v", err)
	}

	compiled, err := CompilePlan(planPath, content, NewParser(nil))
	if err != nil {
		t.Fatalf("CompilePlan failed: %v", err)
	}
	if compiled.IRVersion == "" {
		t.Fatalf("expected ir version")
	}
	if len(compiled.Source.Nodes) == 0 {
		t.Fatalf("expected source nodes")
	}
	if len(compiled.Semantic.Tasks) == 0 {
		t.Fatalf("expected semantic tasks")
	}
	found := false
	for _, task := range compiled.Semantic.Tasks {
		if task.ID == "fixture.mixed.ir" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected semantic task id fixture.mixed.ir, got %#v", compiled.Semantic.Tasks)
	}
}
