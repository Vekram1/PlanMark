package compile

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/build"
	"github.com/vikramoddiraju/planmark/internal/ir"
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
	return compileNodesWithLimits(planPath, content, parser, DefaultLimits())
}

func compileNodesWithLimits(planPath string, content []byte, parser *Parser, limits Limits) ([]CompiledNode, error) {
	if parser == nil {
		parser = NewParser(nil)
	}
	limits = limits.normalized()
	if err := limits.validateContent(planPath, content); err != nil {
		return nil, err
	}

	nodes, err := parser.Parse(planPath, content)
	if err != nil {
		return nil, err
	}
	if err := limits.validateNodeCount(planPath, nodes); err != nil {
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

func CompilePlan(planPath string, content []byte, parser *Parser) (ir.PlanIR, error) {
	return compilePlanWithLimits(planPath, content, parser, DefaultLimits())
}

func compilePlanWithLimits(planPath string, content []byte, parser *Parser, limits Limits) (ir.PlanIR, error) {
	compiled, err := compileNodesWithLimits(planPath, content, parser, limits)
	if err != nil {
		return ir.PlanIR{}, err
	}
	metadata, err := ParseMetadata(content)
	if err != nil {
		return ir.PlanIR{}, err
	}
	attachments := AttachMetadataToNodes(extractNodes(compiled), metadata)
	if len(attachments.Nodes) != len(compiled) {
		return ir.PlanIR{}, fmt.Errorf("attachment/node count mismatch: attachments=%d compiled=%d", len(attachments.Nodes), len(compiled))
	}
	limits = limits.normalized()
	if err := limits.validateMetadataPerNode(planPath, attachments); err != nil {
		return ir.PlanIR{}, err
	}

	occurrenceByBaseRef := make(map[string]int)
	nodeRefs := make([]string, len(compiled))
	sourceNodes := make([]ir.SourceNode, 0, len(compiled))

	for idx, cn := range compiled {
		attached := attachments.Nodes[idx]
		baseRef := fmt.Sprintf("%s|%s|%s", filepath.ToSlash(planPath), cn.Node.Kind, cn.Slice.Hash)
		occurrenceByBaseRef[baseRef]++
		nodeRef := fmt.Sprintf("%s#%d", baseRef, occurrenceByBaseRef[baseRef])
		nodeRefs[idx] = nodeRef

		sourceNode := ir.SourceNode{
			NodeRef:   nodeRef,
			Kind:      string(cn.Node.Kind),
			Line:      cn.Node.Line,
			StartLine: cn.Slice.StartLine,
			EndLine:   cn.Slice.EndLine,
			SliceHash: cn.Slice.Hash,
			SliceText: cn.Slice.Text,
			Text:      cn.Node.Text,
			Checked:   cn.Node.Checked,
		}
		if len(attached.Opaque) > 0 {
			sourceNode.MetaOpaque = make([]ir.Meta, 0, len(attached.Opaque))
			for _, opaque := range attached.Opaque {
				sourceNode.MetaOpaque = append(sourceNode.MetaOpaque, ir.Meta{Key: opaque.Key, Value: opaque.Value, Line: opaque.Line})
			}
		}
		sourceNodes = append(sourceNodes, sourceNode)
	}

	semanticTasks := make([]ir.Task, 0)
	taskCandidates := make([]ir.TaskCandidate, 0)
	parentByIndex := structuralParentIndexes(extractNodes(compiled))
	promotedHeadingByIndex := make(map[int]struct{})
	for idx, attached := range attachments.Nodes {
		if isPromotedHeadingTask(attached.Node, attached.KnownByKey) {
			promotedHeadingByIndex[idx] = struct{}{}
		}
	}

	for idx, attached := range attachments.Nodes {
		if isTaskCandidateNode(idx, attached.Node, attachments.Nodes, parentByIndex, promotedHeadingByIndex) {
			taskCandidates = append(taskCandidates, ir.TaskCandidate{
				NodeRef:   nodeRefs[idx],
				Path:      filepath.ToSlash(planPath),
				StartLine: compiled[idx].Slice.StartLine,
				EndLine:   compiled[idx].Slice.EndLine,
				SliceHash: compiled[idx].Slice.Hash,
				Kind:      string(attached.Node.Kind),
				Title:     strings.TrimSpace(attached.Node.Text),
			})
		}

		switch attached.Node.Kind {
		case NodeKindHeading:
			if _, ok := promotedHeadingByIndex[idx]; !ok {
				continue
			}
		case NodeKindCheckbox:
			if isStepCheckbox(idx, attachments.Nodes, parentByIndex, promotedHeadingByIndex) {
				continue
			}
		default:
			continue
		}

		nodeRef := nodeRefs[idx]
		taskID := firstKnownValue(attached.KnownByKey, "id")
		if taskID == "" {
			taskID = nodeRef
		}
		task := ir.Task{
			ID:      taskID,
			NodeRef: nodeRef,
			Title:   attached.Node.Text,
			Horizon: firstKnownValue(attached.KnownByKey, "horizon"),
			Deps:    splitCSVValues(attached.KnownByKey["deps"]),
			Accept:  valuesOf(attached.KnownByKey["accept"]),
		}
		task.Steps = descendantSteps(idx, attachments.Nodes, compiled, nodeRefs, parentByIndex, promotedHeadingByIndex)
		task.EvidenceNodeRefs = descendantEvidenceNodeRefs(idx, attachments.Nodes, nodeRefs, parentByIndex, promotedHeadingByIndex)
		task.SemanticFingerprint = build.TaskSemanticFingerprint(task)
		semanticTasks = append(semanticTasks, task)
	}

	return ir.PlanIR{
		IRVersion:                       "v0.2",
		DeterminismPolicyVersion:        "v0.1",
		SemanticDerivationPolicyVersion: "v0.1",
		PlanPath:                        filepath.ToSlash(planPath),
		Source:                          ir.SourceIR{Nodes: sourceNodes},
		Semantic: ir.SemanticIR{
			Tasks:          semanticTasks,
			TaskCandidates: taskCandidates,
		},
	}, nil
}

func isTaskCandidateNode(idx int, node Node, attached []AttachedNodeMetadata, parentByIndex map[int]int, promotedHeadingByIndex map[int]struct{}) bool {
	if strings.TrimSpace(node.Text) == "" {
		return false
	}
	switch node.Kind {
	case NodeKindHeading:
		return true
	case NodeKindCheckbox:
		return !isStepCheckbox(idx, attached, parentByIndex, promotedHeadingByIndex)
	default:
		return false
	}
}

func isPromotedHeadingTask(node Node, known map[string][]MetadataEntry) bool {
	if node.Kind != NodeKindHeading {
		return false
	}
	for _, key := range []string{"id", "horizon", "deps", "accept"} {
		if hasKnownMetadataValue(known[key]) {
			return true
		}
	}
	return false
}

func hasKnownMetadataValue(entries []MetadataEntry) bool {
	for _, entry := range entries {
		if strings.TrimSpace(entry.Value) != "" {
			return true
		}
	}
	return false
}

func structuralParentIndexes(nodes []Node) map[int]int {
	parentByIndex := make(map[int]int, len(nodes))
	for childIdx, child := range nodes {
		bestIdx := -1
		bestSpan := 0
		for parentIdx := 0; parentIdx < childIdx; parentIdx++ {
			parent := nodes[parentIdx]
			if !nodeOwnsNode(parent, child) {
				continue
			}
			span := parent.EndLine - parent.StartLine
			if bestIdx == -1 || span < bestSpan || (span == bestSpan && parent.Line > nodes[bestIdx].Line) {
				bestIdx = parentIdx
				bestSpan = span
			}
		}
		parentByIndex[childIdx] = bestIdx
	}
	return parentByIndex
}

func nodeOwnsNode(parent Node, child Node) bool {
	if child.Line <= parent.Line || child.Line > parent.EndLine {
		return false
	}
	switch parent.Kind {
	case NodeKindHeading:
		return true
	case NodeKindCheckbox:
		return child.Indent > parent.Indent
	default:
		return false
	}
}

func isStepCheckbox(idx int, attached []AttachedNodeMetadata, parentByIndex map[int]int, promotedHeadingByIndex map[int]struct{}) bool {
	parentIdx, ok := parentByIndex[idx]
	if !ok || parentIdx < 0 {
		return false
	}
	if attached[parentIdx].Node.Kind == NodeKindCheckbox {
		return true
	}
	if _, ok := promotedHeadingByIndex[parentIdx]; ok {
		return true
	}
	return false
}

func descendantSteps(taskIdx int, attached []AttachedNodeMetadata, compiled []CompiledNode, nodeRefs []string, parentByIndex map[int]int, promotedHeadingByIndex map[int]struct{}) []ir.TaskStep {
	steps := make([]ir.TaskStep, 0)
	for idx, parentIdx := range parentByIndex {
		if parentIdx != taskIdx {
			continue
		}
		if attached[idx].Node.Kind != NodeKindCheckbox {
			continue
		}
		if _, promoted := promotedHeadingByIndex[idx]; promoted {
			continue
		}
		steps = append(steps, ir.TaskStep{
			NodeRef:   nodeRefs[idx],
			Title:     attached[idx].Node.Text,
			Checked:   attached[idx].Node.Checked,
			SliceHash: compiled[idx].Slice.Hash,
		})
	}
	return steps
}

func descendantEvidenceNodeRefs(taskIdx int, attached []AttachedNodeMetadata, nodeRefs []string, parentByIndex map[int]int, promotedHeadingByIndex map[int]struct{}) []string {
	refs := make([]string, 0)
	for idx, parentIdx := range parentByIndex {
		if parentIdx != taskIdx {
			continue
		}
		if attached[idx].Node.Kind == NodeKindCheckbox {
			continue
		}
		if _, promoted := promotedHeadingByIndex[idx]; promoted {
			continue
		}
		refs = append(refs, nodeRefs[idx])
	}
	return refs
}

func extractNodes(compiled []CompiledNode) []Node {
	nodes := make([]Node, 0, len(compiled))
	for _, cn := range compiled {
		nodes = append(nodes, cn.Node)
	}
	return nodes
}

func valuesOf(entries []MetadataEntry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.TrimSpace(entry.Value) == "" {
			continue
		}
		out = append(out, entry.Value)
	}
	return out
}

func splitCSVValues(entries []MetadataEntry) []string {
	values := make([]string, 0)
	for _, entry := range entries {
		for _, part := range strings.Split(entry.Value, ",") {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				values = append(values, trimmed)
			}
		}
	}
	return values
}

func firstKnownValue(known map[string][]MetadataEntry, key string) string {
	entries := known[key]
	if len(entries) == 0 {
		return ""
	}
	return strings.TrimSpace(entries[len(entries)-1].Value)
}

func normalizedLines(content string) []string {
	if content == "" {
		return []string{}
	}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	return strings.Split(content, "\n")
}

func sliceHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
