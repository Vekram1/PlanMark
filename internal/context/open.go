package context

import (
	"fmt"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/ir"
)

type OpenResult struct {
	QueryID    string            `json:"query_id"`
	TaskID     string            `json:"task_id,omitempty"`
	NodeRef    string            `json:"node_ref"`
	Kind       string            `json:"kind"`
	Title      string            `json:"title,omitempty"`
	Steps      []TaskStepContext `json:"steps,omitempty"`
	Evidence   []EvidenceSlice   `json:"evidence,omitempty"`
	SourcePath string            `json:"source_path"`
	StartLine  int               `json:"start_line"`
	EndLine    int               `json:"end_line"`
	SliceHash  string            `json:"slice_hash"`
	SliceText  string            `json:"slice_text"`
}

func Open(plan ir.PlanIR, idOrNodeRef string) (OpenResult, error) {
	target := strings.TrimSpace(idOrNodeRef)
	if target == "" {
		return OpenResult{}, fmt.Errorf("%w: empty id", ErrTaskNotFound)
	}

	nodeByRef := make(map[string]ir.SourceNode, len(plan.Source.Nodes))
	for _, node := range plan.Source.Nodes {
		nodeByRef[node.NodeRef] = node
	}

	for _, task := range plan.Semantic.Tasks {
		if strings.TrimSpace(task.ID) != target {
			continue
		}
		node, ok := nodeByRef[task.NodeRef]
		if !ok {
			return OpenResult{}, fmt.Errorf("source node missing for task %q (node_ref=%s)", task.ID, task.NodeRef)
		}
		return OpenResult{
			QueryID:    target,
			TaskID:     task.ID,
			NodeRef:    task.NodeRef,
			Kind:       node.Kind,
			Title:      task.Title,
			Steps:      buildL0Packet(plan, task, node).Steps,
			Evidence:   buildL0Packet(plan, task, node).Evidence,
			SourcePath: plan.PlanPath,
			StartLine:  node.StartLine,
			EndLine:    node.EndLine,
			SliceHash:  node.SliceHash,
			SliceText:  node.SliceText,
		}, nil
	}

	node, ok := nodeByRef[target]
	if !ok {
		return OpenResult{}, fmt.Errorf("%w: %s", ErrTaskNotFound, target)
	}
	return OpenResult{
		QueryID:    target,
		NodeRef:    node.NodeRef,
		Kind:       node.Kind,
		Title:      node.Text,
		SourcePath: plan.PlanPath,
		StartLine:  node.StartLine,
		EndLine:    node.EndLine,
		SliceHash:  node.SliceHash,
		SliceText:  node.SliceText,
	}, nil
}
