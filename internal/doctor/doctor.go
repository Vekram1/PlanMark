package doctor

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/diag"
	"github.com/vikramoddiraju/planmark/internal/ir"
)

type Result struct {
	Profile     string            `json:"profile"`
	ParsedNodes int               `json:"parsed_nodes"`
	ParsedTasks int               `json:"parsed_tasks"`
	Diagnostics []diag.Diagnostic `json:"diagnostics"`
}

var validProfiles = map[string]struct{}{
	"loose": {},
	"build": {},
	"exec":  {},
}

var validHorizons = map[string]struct{}{
	"now":   {},
	"next":  {},
	"later": {},
}

type QueryTask struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Horizon string   `json:"horizon,omitempty"`
	NodeRef string   `json:"node_ref"`
	Deps    []string `json:"deps,omitempty"`
	Ready   bool     `json:"ready"`
	Blocked bool     `json:"blocked"`
}

func Run(plan ir.PlanIR, profile string) (Result, error) {
	normalized, err := NormalizeProfile(profile)
	if err != nil {
		return Result{}, err
	}

	diagnostics := make([]diag.Diagnostic, 0)
	diagnostics = append(diagnostics, ValidateGraph(plan).Diagnostics...)
	diagnostics = append(diagnostics, EvaluateHorizonReadinessWithProfile(plan, normalized)...)
	diag.Sort(diagnostics)

	return Result{
		Profile:     normalized,
		ParsedNodes: len(plan.Source.Nodes),
		ParsedTasks: len(plan.Semantic.Tasks),
		Diagnostics: diagnostics,
	}, nil
}

func NormalizeProfile(profile string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(profile))
	if normalized == "" {
		normalized = "loose"
	}
	if _, ok := validProfiles[normalized]; !ok {
		return "", errors.New("invalid profile: must be one of loose|build|exec")
	}
	return normalized, nil
}

func NormalizeHorizonFilter(horizon string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(horizon))
	if normalized == "" {
		return "", nil
	}
	if _, ok := validHorizons[normalized]; !ok {
		return "", errors.New("invalid horizon filter: must be one of now|next|later")
	}
	return normalized, nil
}

func QueryTasks(plan ir.PlanIR, horizonFilter string) ([]QueryTask, error) {
	normalizedHorizon, err := NormalizeHorizonFilter(horizonFilter)
	if err != nil {
		return nil, err
	}

	out := make([]QueryTask, 0, len(plan.Semantic.Tasks))
	for _, task := range plan.Semantic.Tasks {
		horizon := strings.ToLower(strings.TrimSpace(task.Horizon))
		if normalizedHorizon != "" && horizon != normalizedHorizon {
			continue
		}

		deps := make([]string, 0, len(task.Deps))
		for _, dep := range task.Deps {
			trimmed := strings.TrimSpace(dep)
			if trimmed == "" {
				continue
			}
			deps = append(deps, trimmed)
		}
		sort.Strings(deps)

		blocked := len(deps) > 0
		out = append(out, QueryTask{
			ID:      task.ID,
			Title:   task.Title,
			Horizon: horizon,
			NodeRef: task.NodeRef,
			Deps:    deps,
			Ready:   !blocked,
			Blocked: blocked,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func ValidateGraph(plan ir.PlanIR) Result {
	diagnostics := make([]diag.Diagnostic, 0)
	tasks := plan.Semantic.Tasks

	firstByID := make(map[string]ir.Task, len(tasks))
	ids := make([]string, 0, len(tasks))

	for _, task := range tasks {
		id := strings.TrimSpace(task.ID)
		if id == "" {
			continue
		}
		if first, exists := firstByID[id]; exists {
			diagnostics = append(diagnostics, diag.Diagnostic{
				Severity: diag.SeverityError,
				Code:     diag.CodeDuplicateTaskID,
				Message:  fmt.Sprintf("duplicate task id %q (node_refs: %s, %s)", id, first.NodeRef, task.NodeRef),
				Source:   diag.SourceSpan{Path: plan.PlanPath},
			})
			continue
		}
		firstByID[id] = task
		ids = append(ids, id)
	}

	sort.Strings(ids)
	for _, id := range ids {
		task := firstByID[id]
		for _, dep := range task.Deps {
			depID := strings.TrimSpace(dep)
			if depID == "" {
				continue
			}
			if _, ok := firstByID[depID]; ok {
				continue
			}
			diagnostics = append(diagnostics, diag.Diagnostic{
				Severity: diag.SeverityError,
				Code:     diag.CodeUnknownDependency,
				Message:  fmt.Sprintf("task %q depends on unknown task id %q", id, depID),
				Source:   diag.SourceSpan{Path: plan.PlanPath},
			})
		}
	}

	diagnostics = append(diagnostics, detectDependencyCycles(plan.PlanPath, firstByID, ids)...)
	diag.Sort(diagnostics)
	return Result{Diagnostics: diagnostics}
}

func detectDependencyCycles(planPath string, tasks map[string]ir.Task, ids []string) []diag.Diagnostic {
	const (
		unvisited = 0
		visiting  = 1
		visited   = 2
	)

	state := make(map[string]int, len(ids))
	stack := make([]string, 0, len(ids))
	stackIndex := make(map[string]int, len(ids))
	seenCycles := make(map[string]struct{})
	diagnostics := make([]diag.Diagnostic, 0)

	var visit func(id string)
	visit = func(id string) {
		state[id] = visiting
		stackIndex[id] = len(stack)
		stack = append(stack, id)

		deps := append([]string(nil), tasks[id].Deps...)
		sort.Strings(deps)
		for _, dep := range deps {
			depID := strings.TrimSpace(dep)
			if depID == "" {
				continue
			}
			if _, ok := tasks[depID]; !ok {
				continue
			}

			switch state[depID] {
			case unvisited:
				visit(depID)
			case visiting:
				start := stackIndex[depID]
				cycle := append([]string(nil), stack[start:]...)
				cycle = append(cycle, depID)
				key := strings.Join(cycle, "->")
				if _, seen := seenCycles[key]; !seen {
					seenCycles[key] = struct{}{}
					diagnostics = append(diagnostics, diag.Diagnostic{
						Severity: diag.SeverityError,
						Code:     diag.CodeDependencyCycle,
						Message:  fmt.Sprintf("dependency cycle detected: %s", strings.Join(cycle, " -> ")),
						Source:   diag.SourceSpan{Path: planPath},
					})
				}
			}
		}

		stack = stack[:len(stack)-1]
		delete(stackIndex, id)
		state[id] = visited
	}

	for _, id := range ids {
		if state[id] == unvisited {
			visit(id)
		}
	}
	return diagnostics
}
