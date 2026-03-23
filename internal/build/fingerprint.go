package build

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/ir"
)

type taskSemanticFingerprintPayload struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	Horizon          string   `json:"horizon,omitempty"`
	Deps             []string `json:"deps,omitempty"`
	Accept           []string `json:"accept,omitempty"`
	StepTitles       []string `json:"step_titles,omitempty"`
	EvidenceNodeRefs []string `json:"evidence_node_refs,omitempty"`
}

func TaskSemanticFingerprint(task ir.Task) string {
	payload := taskSemanticFingerprintPayload{
		ID:               strings.TrimSpace(task.ID),
		Title:            strings.TrimSpace(task.Title),
		Horizon:          strings.ToLower(strings.TrimSpace(task.Horizon)),
		Deps:             normalizeValues(task.Deps),
		Accept:           normalizeValues(task.Accept),
		StepTitles:       orderedStepTitles(task.Steps),
		EvidenceNodeRefs: orderedValues(task.EvidenceNodeRefs),
	}

	data, _ := json.Marshal(payload)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func normalizeValues(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	sort.Strings(normalized)
	return normalized
}

func orderedStepTitles(steps []ir.TaskStep) []string {
	ordered := make([]string, 0, len(steps))
	for _, step := range steps {
		trimmed := strings.TrimSpace(step.Title)
		if trimmed == "" {
			continue
		}
		ordered = append(ordered, trimmed)
	}
	return ordered
}

func orderedValues(values []string) []string {
	ordered := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		ordered = append(ordered, trimmed)
	}
	return ordered
}
