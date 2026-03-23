package context

import (
	"fmt"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/ir"
)

type ExplainBlocker struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

type ExplainResult struct {
	TaskID            string           `json:"task_id"`
	Title             string           `json:"title"`
	Horizon           string           `json:"horizon,omitempty"`
	StepCount         int              `json:"step_count,omitempty"`
	EvidenceNodeRefs  []string         `json:"evidence_node_refs,omitempty"`
	Runnable          bool             `json:"runnable"`
	Blockers          []ExplainBlocker `json:"blockers,omitempty"`
	SuggestedMetadata []string         `json:"suggested_metadata,omitempty"`
}

func Explain(plan ir.PlanIR, taskID string) (ExplainResult, error) {
	requestedID := strings.TrimSpace(taskID)
	if requestedID == "" {
		return ExplainResult{}, fmt.Errorf("%w: empty id", ErrTaskNotFound)
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
		return ExplainResult{}, fmt.Errorf("%w: %s", ErrTaskNotFound, requestedID)
	}

	result := ExplainResult{
		TaskID:           task.ID,
		Title:            task.Title,
		Horizon:          task.Horizon,
		StepCount:        len(task.Steps),
		EvidenceNodeRefs: append([]string(nil), task.EvidenceNodeRefs...),
		Runnable:         true,
	}

	taskByID := make(map[string]struct{}, len(plan.Semantic.Tasks))
	for _, candidate := range plan.Semantic.Tasks {
		taskByID[strings.TrimSpace(candidate.ID)] = struct{}{}
	}

	if strings.EqualFold(strings.TrimSpace(task.Horizon), "now") {
		if !hasNonEmpty(task.Accept) {
			result.Blockers = append(result.Blockers, ExplainBlocker{
				Code:       "MISSING_ACCEPT",
				Message:    fmt.Sprintf("horizon=now task %q requires at least one @accept", task.ID),
				Suggestion: "@accept cmd:<command>",
			})
			result.SuggestedMetadata = append(result.SuggestedMetadata, "@accept cmd:<command>")
		}
		for _, dep := range task.Deps {
			depID := strings.TrimSpace(dep)
			if depID == "" {
				continue
			}
			if _, ok := taskByID[depID]; !ok {
				result.Blockers = append(result.Blockers, ExplainBlocker{
					Code:    "UNKNOWN_DEPENDENCY",
					Message: fmt.Sprintf("task %q depends on unknown task id %q", task.ID, depID),
				})
			}
		}
	}

	result.Runnable = len(result.Blockers) == 0
	return result, nil
}
