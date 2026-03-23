package compile

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

type NodeKind string

const (
	NodeKindHeading  NodeKind = "heading"
	NodeKindCheckbox NodeKind = "checkbox"
)

type Node struct {
	Kind      NodeKind
	Line      int
	StartLine int
	EndLine   int
	Level     int
	Indent    int
	Text      string
	Checked   bool
}

type ParserBackend interface {
	ExtractNodes(planPath string, content []byte) ([]Node, error)
}

type Parser struct {
	backend ParserBackend
}

func NewParser(backend ParserBackend) *Parser {
	if backend == nil {
		backend = NewMarkdownLineBackend()
	}
	return &Parser{backend: backend}
}

func (p *Parser) Parse(planPath string, content []byte) ([]Node, error) {
	return p.backend.ExtractNodes(planPath, content)
}

type MarkdownLineBackend struct{}

func NewMarkdownLineBackend() *MarkdownLineBackend {
	return &MarkdownLineBackend{}
}

var (
	headingPattern  = regexp.MustCompile(`^\s*(#{1,6})\s+(.+?)\s*$`)
	checkboxPattern = regexp.MustCompile(`^\s*-\s\[( |x|X)\]\s+(.+?)\s*$`)
	fencePattern    = regexp.MustCompile(`^\s*(` + "```" + `|~~~)`)
)

func (b *MarkdownLineBackend) ExtractNodes(planPath string, content []byte) ([]Node, error) {
	if len(content) == 0 {
		return []Node{}, nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	lines := make([]string, 0)
	nodes := make([]Node, 0)
	inFence := false

	for scanner.Scan() {
		lineNo++
		line := strings.TrimRight(scanner.Text(), "\r")
		lines = append(lines, line)
		indent := leadingIndentWidth(line)

		if fencePattern.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}

		if m := headingPattern.FindStringSubmatch(line); m != nil {
			nodes = append(nodes, Node{
				Kind:      NodeKindHeading,
				Line:      lineNo,
				StartLine: lineNo,
				EndLine:   lineNo,
				Level:     len(m[1]),
				Indent:    indent,
				Text:      strings.TrimSpace(m[2]),
			})
			continue
		}

		if m := checkboxPattern.FindStringSubmatch(line); m != nil {
			nodes = append(nodes, Node{
				Kind:      NodeKindCheckbox,
				Line:      lineNo,
				StartLine: lineNo,
				EndLine:   lineNo,
				Indent:    indent,
				Text:      strings.TrimSpace(m[2]),
				Checked:   strings.EqualFold(m[1], "x"),
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", planPath, err)
	}
	return applyStructuralScopes(nodes, lines), nil
}

func leadingIndentWidth(line string) int {
	width := 0
	for _, r := range line {
		switch r {
		case ' ':
			width++
		case '\t':
			width += 4
		default:
			return width
		}
	}
	return width
}

func applyStructuralScopes(nodes []Node, lines []string) []Node {
	if len(nodes) == 0 {
		return nodes
	}

	scoped := append([]Node(nil), nodes...)
	nodeIndexByLine := make(map[int]int, len(scoped))
	for i, node := range scoped {
		nodeIndexByLine[node.Line] = i
	}

	for i := range scoped {
		switch scoped[i].Kind {
		case NodeKindHeading:
			scoped[i].EndLine = headingScopeEnd(scoped, i, len(lines))
		case NodeKindCheckbox:
			scoped[i].EndLine = checkboxScopeEnd(scoped, i, lines, nodeIndexByLine)
		}
	}

	return scoped
}

func headingScopeEnd(nodes []Node, idx int, totalLines int) int {
	node := nodes[idx]
	end := totalLines
	for j := idx + 1; j < len(nodes); j++ {
		candidate := nodes[j]
		if candidate.Kind != NodeKindHeading {
			continue
		}
		if candidate.Level <= node.Level {
			end = candidate.Line - 1
			break
		}
	}
	if end < node.Line {
		return node.Line
	}
	return end
}

func checkboxScopeEnd(nodes []Node, idx int, lines []string, nodeIndexByLine map[int]int) int {
	node := nodes[idx]
	end := node.Line
	line := node.Line + 1
	for line <= len(lines) {
		if nextNodeIdx, ok := nodeIndexByLine[line]; ok {
			nextNode := nodes[nextNodeIdx]
			if nextNode.Kind == NodeKindHeading || nextNode.Indent <= node.Indent {
				break
			}
		}

		current := lines[line-1]
		if strings.TrimSpace(current) == "" {
			nextOwned, ok := nextOwnedContentLine(line+1, lines, nodes, node, nodeIndexByLine)
			if !ok {
				break
			}
			end = nextOwned
			line = nextOwned + 1
			continue
		}

		if leadingIndentWidth(current) <= node.Indent {
			break
		}

		end = line
		line++
	}
	if end < node.Line {
		return node.Line
	}
	return end
}

func nextOwnedContentLine(start int, lines []string, nodes []Node, owner Node, nodeIndexByLine map[int]int) (int, bool) {
	for line := start; line <= len(lines); line++ {
		if nextNodeIdx, ok := nodeIndexByLine[line]; ok {
			nextNode := nodes[nextNodeIdx]
			if nextNode.Kind == NodeKindHeading || nextNode.Indent <= owner.Indent {
				return 0, false
			}
		}
		text := lines[line-1]
		if strings.TrimSpace(text) == "" {
			continue
		}
		if leadingIndentWidth(text) <= owner.Indent {
			return 0, false
		}
		return line, true
	}
	return 0, false
}
