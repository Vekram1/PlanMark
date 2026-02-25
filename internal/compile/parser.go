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
	nodes := make([]Node, 0)
	inFence := false

	for scanner.Scan() {
		lineNo++
		line := strings.TrimRight(scanner.Text(), "\r")

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
				Text:      strings.TrimSpace(m[2]),
				Checked:   strings.EqualFold(m[1], "x"),
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", planPath, err)
	}
	return nodes, nil
}
