package compile

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

type SourceSlice struct {
	StartLine int
	EndLine   int
	Text      string
	Hash      string
}

type CompiledNode struct {
	Node  Node
	Slice SourceSlice
}

func CompileNodes(planPath string, content []byte, parser *Parser) ([]CompiledNode, error) {
	if parser == nil {
		parser = NewParser(nil)
	}

	nodes, err := parser.Parse(planPath, content)
	if err != nil {
		return nil, err
	}

	lines := normalizedLines(string(content))
	compiled := make([]CompiledNode, 0, len(nodes))

	for _, n := range nodes {
		if n.StartLine <= 0 || n.EndLine < n.StartLine || n.EndLine > len(lines) {
			return nil, fmt.Errorf("invalid source range for node %q: %d-%d", n.Text, n.StartLine, n.EndLine)
		}

		sliceText := strings.Join(lines[n.StartLine-1:n.EndLine], "\n")
		compiled = append(compiled, CompiledNode{
			Node: n,
			Slice: SourceSlice{
				StartLine: n.StartLine,
				EndLine:   n.EndLine,
				Text:      sliceText,
				Hash:      sliceHash(sliceText),
			},
		})
	}

	return compiled, nil
}

func normalizedLines(content string) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	return strings.Split(content, "\n")
}

func sliceHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
