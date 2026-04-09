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
	ID               string                   `json:"id"`
	Title            string                   `json:"title"`
	CanonicalStatus  string                   `json:"canonical_status"`
	Horizon          string                   `json:"horizon,omitempty"`
	Deps             []string                 `json:"deps,omitempty"`
	Accept           []string                 `json:"accept,omitempty"`
	Sections         []taskSectionFingerprint `json:"sections,omitempty"`
	Steps            []taskStepFingerprint    `json:"steps,omitempty"`
	EvidenceNodeRefs []string                 `json:"evidence_node_refs,omitempty"`
}

type taskSectionFingerprint struct {
	Key   string   `json:"key,omitempty"`
	Title string   `json:"title,omitempty"`
	Body  []string `json:"body,omitempty"`
}

type taskStepFingerprint struct {
	NodeRef   string `json:"node_ref,omitempty"`
	Title     string `json:"title"`
	Checked   bool   `json:"checked,omitempty"`
	SliceHash string `json:"slice_hash,omitempty"`
}

func TaskSemanticFingerprint(task ir.Task) string {
	payload := taskSemanticFingerprintPayload{
		ID:               strings.TrimSpace(task.ID),
		Title:            strings.TrimSpace(task.Title),
		CanonicalStatus:  normalizeCanonicalStatus(task.CanonicalStatus),
		Horizon:          strings.ToLower(strings.TrimSpace(task.Horizon)),
		Deps:             normalizeValues(task.Deps),
		Accept:           normalizeValues(task.Accept),
		Sections:         orderedSections(task.Sections),
		Steps:            orderedSteps(task.Steps),
		EvidenceNodeRefs: orderedValues(task.EvidenceNodeRefs),
	}

	data, _ := json.Marshal(payload)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func normalizeCanonicalStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "done":
		return "done"
	default:
		return "open"
	}
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

func orderedSections(sections []ir.TaskSection) []taskSectionFingerprint {
	fingerprints := make([]taskSectionFingerprint, 0, len(sections))
	for _, section := range sections {
		body := normalizedSectionBody(section.Body)
		key := strings.TrimSpace(section.Key)
		title := strings.TrimSpace(section.Title)
		if key == "" && title == "" && len(body) == 0 {
			continue
		}
		fingerprints = append(fingerprints, taskSectionFingerprint{
			Key:   key,
			Title: title,
			Body:  body,
		})
	}
	return fingerprints
}

func normalizedSectionBody(lines []string) []string {
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	normalized := make([]string, 0, end-start)
	lastBlank := false
	for _, line := range lines[start:end] {
		trimmedRight := strings.TrimRight(line, " \t")
		isBlank := strings.TrimSpace(trimmedRight) == ""
		if isBlank {
			if lastBlank {
				continue
			}
			lastBlank = true
			normalized = append(normalized, "")
			continue
		}
		lastBlank = false
		normalized = append(normalized, trimmedRight)
	}
	return normalized
}

func orderedSteps(steps []ir.TaskStep) []taskStepFingerprint {
	fingerprints := make([]taskStepFingerprint, 0, len(steps))
	for _, step := range steps {
		title := strings.TrimSpace(step.Title)
		if title == "" {
			continue
		}
		fingerprints = append(fingerprints, taskStepFingerprint{
			NodeRef:   strings.TrimSpace(step.NodeRef),
			Title:     title,
			Checked:   step.Checked,
			SliceHash: strings.TrimSpace(step.SliceHash),
		})
	}
	return fingerprints
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
