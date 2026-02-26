package change

import (
	"sort"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/ir"
)

func SemanticDiff(previous ir.PlanIR, current ir.PlanIR) []TaskChange {
	prevByID := make(map[string]ir.Task, len(previous.Semantic.Tasks))
	currByID := make(map[string]ir.Task, len(current.Semantic.Tasks))
	for _, task := range previous.Semantic.Tasks {
		prevByID[strings.TrimSpace(task.ID)] = task
	}
	for _, task := range current.Semantic.Tasks {
		currByID[strings.TrimSpace(task.ID)] = task
	}

	supersedesByNodeRef := supersedesMap(current)
	consumedPrev := make(map[string]struct{})
	changes := make([]TaskChange, 0)

	// explicit identity evolution: @supersedes old-id -> moved
	currIDs := sortedTaskIDs(current.Semantic.Tasks)
	for _, id := range currIDs {
		curr := currByID[id]
		supersedes := strings.TrimSpace(supersedesByNodeRef[curr.NodeRef])
		if supersedes == "" || supersedes == id {
			continue
		}
		if _, ok := prevByID[supersedes]; ok {
			changes = append(changes, TaskChange{
				Class: ClassMoved,
				OldID: supersedes,
				NewID: id,
			})
			consumedPrev[supersedes] = struct{}{}
		}
	}

	// unchanged/modified/additions
	for _, id := range currIDs {
		curr := currByID[id]
		if prev, ok := prevByID[id]; ok {
			if strings.TrimSpace(prev.SemanticFingerprint) == strings.TrimSpace(curr.SemanticFingerprint) {
				continue
			}
			changes = append(changes, TaskChange{
				Class:  classifyModification(prev, curr),
				TaskID: id,
			})
			continue
		}

		// do not double-report moved targets as added
		supersedes := strings.TrimSpace(supersedesByNodeRef[curr.NodeRef])
		if supersedes != "" {
			if _, movedFromKnown := prevByID[supersedes]; movedFromKnown {
				continue
			}
		}
		changes = append(changes, TaskChange{
			Class:  ClassAdded,
			TaskID: id,
		})
	}

	// deletions
	prevIDs := sortedTaskIDs(previous.Semantic.Tasks)
	for _, id := range prevIDs {
		if _, stillPresent := currByID[id]; stillPresent {
			continue
		}
		if _, moved := consumedPrev[id]; moved {
			continue
		}
		changes = append(changes, TaskChange{
			Class:  ClassDeleted,
			TaskID: id,
		})
	}

	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Class != changes[j].Class {
			return changes[i].Class < changes[j].Class
		}
		if changes[i].TaskID != changes[j].TaskID {
			return changes[i].TaskID < changes[j].TaskID
		}
		if changes[i].OldID != changes[j].OldID {
			return changes[i].OldID < changes[j].OldID
		}
		return changes[i].NewID < changes[j].NewID
	})
	return changes
}

func supersedesMap(plan ir.PlanIR) map[string]string {
	byRef := make(map[string]string)
	for _, node := range plan.Source.Nodes {
		for _, meta := range node.MetaOpaque {
			if strings.EqualFold(strings.TrimSpace(meta.Key), "supersedes") {
				byRef[node.NodeRef] = strings.TrimSpace(meta.Value)
				break
			}
		}
	}
	return byRef
}

func sortedTaskIDs(tasks []ir.Task) []string {
	ids := make([]string, 0, len(tasks))
	seen := make(map[string]struct{}, len(tasks))
	for _, task := range tasks {
		id := strings.TrimSpace(task.ID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
