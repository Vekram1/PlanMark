package compile

import (
	"fmt"
)

type LimitKind string

const (
	LimitKindFileBytes            LimitKind = "file_bytes"
	LimitKindLineBytes            LimitKind = "line_bytes"
	LimitKindNodeCount            LimitKind = "node_count"
	LimitKindMetadataPerNodeLines LimitKind = "metadata_per_node_lines"
)

type Limits struct {
	MaxFileBytes            int
	MaxLineBytes            int
	MaxNodes                int
	MaxMetadataLinesPerNode int
}

func DefaultLimits() Limits {
	return Limits{
		MaxFileBytes:            8 * 1024 * 1024,
		MaxLineBytes:            256 * 1024,
		MaxNodes:                50_000,
		MaxMetadataLinesPerNode: 128,
	}
}

func (l Limits) normalized() Limits {
	defaults := DefaultLimits()
	if l.MaxFileBytes <= 0 {
		l.MaxFileBytes = defaults.MaxFileBytes
	}
	if l.MaxLineBytes <= 0 {
		l.MaxLineBytes = defaults.MaxLineBytes
	}
	if l.MaxNodes <= 0 {
		l.MaxNodes = defaults.MaxNodes
	}
	if l.MaxMetadataLinesPerNode <= 0 {
		l.MaxMetadataLinesPerNode = defaults.MaxMetadataLinesPerNode
	}
	return l
}

type LimitError struct {
	Path   string
	Kind   LimitKind
	Line   int
	Actual int
	Limit  int
}

func (e *LimitError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("compile limit exceeded: kind=%s path=%s line=%d actual=%d limit=%d", e.Kind, e.Path, e.Line, e.Actual, e.Limit)
	}
	return fmt.Sprintf("compile limit exceeded: kind=%s path=%s actual=%d limit=%d", e.Kind, e.Path, e.Actual, e.Limit)
}

func (l Limits) validateContent(planPath string, content []byte) error {
	if len(content) > l.MaxFileBytes {
		return &LimitError{
			Path:   planPath,
			Kind:   LimitKindFileBytes,
			Actual: len(content),
			Limit:  l.MaxFileBytes,
		}
	}
	lines := normalizedLines(string(content))
	for idx, line := range lines {
		if len([]byte(line)) > l.MaxLineBytes {
			return &LimitError{
				Path:   planPath,
				Kind:   LimitKindLineBytes,
				Line:   idx + 1,
				Actual: len([]byte(line)),
				Limit:  l.MaxLineBytes,
			}
		}
	}
	return nil
}

func (l Limits) validateNodeCount(planPath string, nodes []Node) error {
	if len(nodes) > l.MaxNodes {
		return &LimitError{
			Path:   planPath,
			Kind:   LimitKindNodeCount,
			Actual: len(nodes),
			Limit:  l.MaxNodes,
		}
	}
	return nil
}

func (l Limits) validateMetadataPerNode(planPath string, attachments MetadataAttachmentResult) error {
	for _, attached := range attachments.Nodes {
		count := len(attached.Opaque)
		for _, entries := range attached.KnownByKey {
			count += len(entries)
		}
		if count > l.MaxMetadataLinesPerNode {
			return &LimitError{
				Path:   planPath,
				Kind:   LimitKindMetadataPerNodeLines,
				Line:   attached.Node.Line,
				Actual: count,
				Limit:  l.MaxMetadataLinesPerNode,
			}
		}
	}
	return nil
}
