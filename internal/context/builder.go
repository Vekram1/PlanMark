package context

import (
	"errors"
	"fmt"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/ir"
)

var (
	ErrTaskNotFound = errors.New("task not found")
	ErrTaskNotReady = errors.New("task not ready for L0")
)

type L0Packet struct {
	Level      string   `json:"level"`
	TaskID     string   `json:"task_id"`
	NodeRef    string   `json:"node_ref"`
	Title      string   `json:"title"`
	Horizon    string   `json:"horizon,omitempty"`
	Deps       []string `json:"deps,omitempty"`
	Accept     []string `json:"accept,omitempty"`
	SourcePath string   `json:"source_path"`
	StartLine  int      `json:"start_line"`
	EndLine    int      `json:"end_line"`
	SliceHash  string   `json:"slice_hash"`
	SliceText  string   `json:"slice_text"`
}

func BuildL0(plan ir.PlanIR, taskID string) (L0Packet, error) {
	requestedID := strings.TrimSpace(taskID)
	if requestedID == "" {
		return L0Packet{}, fmt.Errorf("%w: empty id", ErrTaskNotFound)
	}

	var task ir.Task
	foundTask := false
	for _, candidate := range plan.Semantic.Tasks {
		if strings.TrimSpace(candidate.ID) == requestedID {
			task = candidate
			foundTask = true
			break
		}
	}
	if !foundTask {
		return L0Packet{}, fmt.Errorf("%w: %s", ErrTaskNotFound, requestedID)
	}

	nodeByRef := make(map[string]ir.SourceNode, len(plan.Source.Nodes))
	for _, node := range plan.Source.Nodes {
		nodeByRef[node.NodeRef] = node
	}

	node, ok := nodeByRef[task.NodeRef]
	if !ok {
		return L0Packet{}, fmt.Errorf("source node missing for task %q (node_ref=%s)", task.ID, task.NodeRef)
	}

	if strings.EqualFold(strings.TrimSpace(task.Horizon), "now") && !hasNonEmpty(task.Accept) {
		return L0Packet{}, fmt.Errorf("%w: horizon=now task %q requires at least one @accept", ErrTaskNotReady, task.ID)
	}

	if strings.EqualFold(strings.TrimSpace(task.Horizon), "now") {
		taskByID := make(map[string]struct{}, len(plan.Semantic.Tasks))
		for _, candidate := range plan.Semantic.Tasks {
			taskByID[strings.TrimSpace(candidate.ID)] = struct{}{}
		}
		for _, dep := range task.Deps {
			depID := strings.TrimSpace(dep)
			if depID == "" {
				continue
			}
			if _, exists := taskByID[depID]; !exists {
				return L0Packet{}, fmt.Errorf("%w: horizon=now task %q has unresolved dependency %q", ErrTaskNotReady, task.ID, depID)
			}
		}
	}

	return L0Packet{
		Level:      "L0",
		TaskID:     task.ID,
		NodeRef:    task.NodeRef,
		Title:      task.Title,
		Horizon:    task.Horizon,
		Deps:       append([]string(nil), task.Deps...),
		Accept:     append([]string(nil), task.Accept...),
		SourcePath: plan.PlanPath,
		StartLine:  node.StartLine,
		EndLine:    node.EndLine,
		SliceHash:  node.SliceHash,
		SliceText:  node.SliceText,
	}, nil
}

func hasNonEmpty(values []string) bool {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}
