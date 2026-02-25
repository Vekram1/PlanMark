package compile

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestParser(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "testdata", "plans", "parser-order.md")
	goldenPath := filepath.Join("..", "..", "testdata", "golden", "parser-order.golden")

	fixture, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	p := NewParser(NewMarkdownLineBackend())
	nodes, err := p.Parse(fixturePath, fixture)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	got := make([]string, 0, len(nodes))
	for _, n := range nodes {
		switch n.Kind {
		case NodeKindHeading:
			got = append(got, "heading|"+itoa(n.Line)+"|"+itoa(n.Level)+"|"+n.Text)
		case NodeKindCheckbox:
			checked := "0"
			if n.Checked {
				checked = "1"
			}
			got = append(got, "checkbox|"+itoa(n.Line)+"|"+checked+"|"+n.Text)
		default:
			t.Fatalf("unexpected node kind: %q", n.Kind)
		}
	}

	goldenRaw, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	want := splitNonEmptyLines(string(goldenRaw))

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("node order mismatch\nwant:\n%s\ngot:\n%s", strings.Join(want, "\n"), strings.Join(got, "\n"))
	}
}

func TestParserIgnoresFencedCodeBlocks(t *testing.T) {
	src := strings.Join([]string{
		"# Top",
		"```md",
		"## NotAHeading",
		"- [ ] not-a-task",
		"```",
		"## RealHeading",
		"- [x] real-task",
	}, "\n")

	p := NewParser(NewMarkdownLineBackend())
	nodes, err := p.Parse("inline.md", []byte(src))
	if err != nil {
		t.Fatalf("parse inline fixture: %v", err)
	}

	got := make([]string, 0, len(nodes))
	for _, n := range nodes {
		switch n.Kind {
		case NodeKindHeading:
			got = append(got, "heading|"+itoa(n.Line)+"|"+itoa(n.Level)+"|"+n.Text)
		case NodeKindCheckbox:
			checked := "0"
			if n.Checked {
				checked = "1"
			}
			got = append(got, "checkbox|"+itoa(n.Line)+"|"+checked+"|"+n.Text)
		default:
			t.Fatalf("unexpected node kind: %q", n.Kind)
		}
	}

	want := []string{
		"heading|1|1|Top",
		"heading|6|2|RealHeading",
		"checkbox|7|1|real-task",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fence handling mismatch\nwant:\n%s\ngot:\n%s", strings.Join(want, "\n"), strings.Join(got, "\n"))
	}
}

func TestParserSupportsLongLines(t *testing.T) {
	longText := strings.Repeat("a", 200000)
	src := "- [ ] " + longText + "\n"

	p := NewParser(NewMarkdownLineBackend())
	nodes, err := p.Parse("long-line.md", []byte(src))
	if err != nil {
		t.Fatalf("parse long line: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].Text != longText {
		t.Fatalf("long line text mismatch")
	}
}

func splitNonEmptyLines(s string) []string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func itoa(v int) string {
	return strconv.Itoa(v)
}
