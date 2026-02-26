package change

import (
	"sort"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/ir"
)

const (
	ClassAdded          = "added"
	ClassDeleted        = "deleted"
	ClassModified       = "modified"
	ClassMoved          = "moved"
	ClassMetadataChange = "metadata_changed"
	ClassDepsChange     = "deps_changed"
	ClassAcceptChange   = "accept_changed"
	ClassHorizonChange  = "horizon_changed"
)

type TaskChange struct {
	Class  string `json:"class"`
	TaskID string `json:"task_id,omitempty"`
	OldID  string `json:"old_id,omitempty"`
	NewID  string `json:"new_id,omitempty"`
}

func classifyModification(before ir.Task, after ir.Task) string {
	beforeHorizon := strings.TrimSpace(strings.ToLower(before.Horizon))
	afterHorizon := strings.TrimSpace(strings.ToLower(after.Horizon))
	if beforeHorizon != afterHorizon {
		return ClassHorizonChange
	}
	if !equalStringSets(before.Deps, after.Deps) {
		return ClassDepsChange
	}
	if !equalStringSets(before.Accept, after.Accept) {
		return ClassAcceptChange
	}
	if strings.TrimSpace(before.Title) != strings.TrimSpace(after.Title) {
		return ClassMetadataChange
	}
	return ClassModified
}

func equalStringSets(a []string, b []string) bool {
	aa := normalizeStringSet(a)
	bb := normalizeStringSet(b)
	if len(aa) != len(bb) {
		return false
	}
	for i := range aa {
		if aa[i] != bb[i] {
			return false
		}
	}
	return true
}

func normalizeStringSet(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
}
