package doctor

import (
	"sort"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/diag"
	"github.com/vikramoddiraju/planmark/internal/ir"
)

type FixOut struct {
	SchemaVersion string          `json:"schema_version"`
	PlanPath      string          `json:"plan_path"`
	Profile       string          `json:"profile"`
	Fixes         []FixSuggestion `json:"fixes"`
}

type FixSuggestion struct {
	TaskID            string    `json:"task_id"`
	Code              diag.Code `json:"code"`
	SuggestedMetadata []string  `json:"suggested_metadata,omitempty"`
	Reason            string    `json:"reason"`
}

func BuildFixOut(plan ir.PlanIR, profile string) FixOut {
	taskByID := make(map[string]ir.Task, len(plan.Semantic.Tasks))
	taskIDs := make([]string, 0, len(plan.Semantic.Tasks))
	seen := make(map[string]struct{}, len(plan.Semantic.Tasks))
	for _, task := range plan.Semantic.Tasks {
		id := strings.TrimSpace(task.ID)
		if id == "" {
			continue
		}
		taskByID[id] = task
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		taskIDs = append(taskIDs, id)
	}
	sort.Strings(taskIDs)

	suggestions := make([]FixSuggestion, 0)
	for _, id := range taskIDs {
		task := taskByID[id]
		if strings.EqualFold(strings.TrimSpace(task.Horizon), "now") && !hasNonEmpty(task.Accept) {
			suggestions = append(suggestions, FixSuggestion{
				TaskID:            task.ID,
				Code:              diag.CodeMissingAccept,
				SuggestedMetadata: []string{"@accept cmd:<command>"},
				Reason:            "horizon=now tasks require at least one @accept",
			})
		}
	}

	return FixOut{
		SchemaVersion: "v0.1",
		PlanPath:      plan.PlanPath,
		Profile:       strings.ToLower(strings.TrimSpace(profile)),
		Fixes:         suggestions,
	}
}
