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

func TestParserBackendConformance(t *testing.T) {
	base := filepath.Join("..", "..", "testdata", "conformance")

	mixedWant := []string{
		"heading|1|1|Plan",
		"checkbox|3|0|first task",
		"heading|8|2|Phase 1",
		"checkbox|9|1|done task",
		"checkbox|10|0|next task",
	}

	for _, tc := range []struct {
		name    string
		fixture string
		want    []string
	}{
		{
			name:    "mixed-lf",
			fixture: "parser_mixed_lf.md",
			want:    mixedWant,
		},
		{
			name:    "mixed-crlf",
			fixture: "parser_mixed_crlf.md",
			want:    mixedWant,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(base, tc.fixture))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}

			nodes, err := NewParser(NewMarkdownLineBackend()).Parse(tc.fixture, content)
			if err != nil {
				t.Fatalf("parse fixture: %v", err)
			}

			got := nodeSignatures(nodes)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("conformance mismatch for %s\nwant:\n%s\ngot:\n%s", tc.fixture, strings.Join(tc.want, "\n"), strings.Join(got, "\n"))
			}
		})
	}

	nfcContent, err := os.ReadFile(filepath.Join(base, "unicode_nfc.md"))
	if err != nil {
		t.Fatalf("read unicode_nfc fixture: %v", err)
	}
	nfdContent, err := os.ReadFile(filepath.Join(base, "unicode_nfd.md"))
	if err != nil {
		t.Fatalf("read unicode_nfd fixture: %v", err)
	}

	nfcNodes, err := NewParser(NewMarkdownLineBackend()).Parse("unicode_nfc.md", nfcContent)
	if err != nil {
		t.Fatalf("parse unicode_nfc fixture: %v", err)
	}
	nfdNodes, err := NewParser(NewMarkdownLineBackend()).Parse("unicode_nfd.md", nfdContent)
	if err != nil {
		t.Fatalf("parse unicode_nfd fixture: %v", err)
	}

	if len(nfcNodes) != 2 || len(nfdNodes) != 2 {
		t.Fatalf("expected 2 nodes for unicode fixtures, got nfc=%d nfd=%d", len(nfcNodes), len(nfdNodes))
	}
	if nfcNodes[0].Text == nfdNodes[0].Text {
		t.Fatalf("expected Unicode forms to remain distinct; got equal heading text %q", nfcNodes[0].Text)
	}
	if nfcNodes[1].Text == nfdNodes[1].Text {
		t.Fatalf("expected Unicode forms to remain distinct; got equal checkbox text %q", nfcNodes[1].Text)
	}
}

func nodeSignatures(nodes []Node) []string {
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		switch n.Kind {
		case NodeKindHeading:
			out = append(out, "heading|"+itoa(n.Line)+"|"+itoa(n.Level)+"|"+n.Text)
		case NodeKindCheckbox:
			checked := "0"
			if n.Checked {
				checked = "1"
			}
			out = append(out, "checkbox|"+itoa(n.Line)+"|"+checked+"|"+n.Text)
		default:
			out = append(out, "unknown|"+itoa(n.Line)+"|"+string(n.Kind)+"|"+n.Text)
		}
	}
	return out
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
