package tracker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

type TaskProjectionStep struct {
	NodeRef string
	Title   string
	Checked bool
}

type TaskProvenance struct {
	NodeRef    string
	Path       string
	StartLine  int
	EndLine    int
	SourceHash string
	CompileID  string
}

type TaskProjectionEvidence struct {
	NodeRef string
	Kind    string
}

type TaskProjectionSection struct {
	Key   string
	Title string
	Body  []string
}

// TaskProjectionV2 is the canonical tracker-neutral task payload projected from PLAN/IR.
type TaskProjectionV2 struct {
	ID                string
	Title             string
	Horizon           string
	Anchor            string
	Provenance        TaskProvenance
	Dependencies      []string
	Acceptance        []string
	Steps             []TaskProjectionStep
	Evidence          []TaskProjectionEvidence
	Sections          []TaskProjectionSection
	ProjectionVersion string
}

// TaskProjection remains the adapter boundary name while the richer v2 shape settles.
type TaskProjection = TaskProjectionV2

type canonicalTaskProjection struct {
	ID                string                   `json:"id"`
	Title             string                   `json:"title,omitempty"`
	Horizon           string                   `json:"horizon,omitempty"`
	Anchor            string                   `json:"anchor,omitempty"`
	Provenance        TaskProvenance           `json:"provenance"`
	Dependencies      []string                 `json:"dependencies,omitempty"`
	Acceptance        []string                 `json:"acceptance,omitempty"`
	Steps             []TaskProjectionStep     `json:"steps,omitempty"`
	Evidence          []TaskProjectionEvidence `json:"evidence,omitempty"`
	Sections          []TaskProjectionSection  `json:"sections,omitempty"`
	ProjectionVersion string                   `json:"projection_version,omitempty"`
}

// TaskProjectionHash computes a deterministic hash over the full tracker-neutral
// task projection. This is intentionally broader than any individual adapter's
// rendered payload so sync planning notices semantic projection changes even
// before every adapter consumes every field.
func TaskProjectionHash(task TaskProjection) (string, error) {
	canonical := canonicalizeTaskProjection(task)
	blob, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("marshal task projection: %w", err)
	}
	sum := sha256.Sum256(blob)
	return hex.EncodeToString(sum[:]), nil
}

func canonicalizeTaskProjection(task TaskProjection) canonicalTaskProjection {
	return canonicalTaskProjection{
		ID:                strings.TrimSpace(task.ID),
		Title:             strings.TrimSpace(task.Title),
		Horizon:           strings.TrimSpace(task.Horizon),
		Anchor:            strings.TrimSpace(task.Anchor),
		Provenance:        normalizedProvenance(task.Provenance),
		Dependencies:      normalizedOrderedStrings(task.Dependencies),
		Acceptance:        normalizedOrderedStrings(task.Acceptance),
		Steps:             normalizedSteps(task.Steps),
		Evidence:          normalizedEvidence(task.Evidence),
		Sections:          normalizedSections(task.Sections),
		ProjectionVersion: normalizedProjectionVersion(task.ProjectionVersion),
	}
}

func normalizedProjectionVersion(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ProjectionSchemaVersionV02
	}
	return trimmed
}

func normalizedOrderedStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func normalizedSteps(steps []TaskProjectionStep) []TaskProjectionStep {
	out := make([]TaskProjectionStep, 0, len(steps))
	for _, step := range steps {
		title := strings.TrimSpace(step.Title)
		nodeRef := strings.TrimSpace(step.NodeRef)
		if title == "" && nodeRef == "" && !step.Checked {
			continue
		}
		out = append(out, TaskProjectionStep{
			NodeRef: nodeRef,
			Title:   title,
			Checked: step.Checked,
		})
	}
	return out
}

func normalizedEvidence(evidence []TaskProjectionEvidence) []TaskProjectionEvidence {
	out := make([]TaskProjectionEvidence, 0, len(evidence))
	for _, item := range evidence {
		nodeRef := strings.TrimSpace(item.NodeRef)
		kind := strings.TrimSpace(item.Kind)
		if nodeRef == "" && kind == "" {
			continue
		}
		out = append(out, TaskProjectionEvidence{
			NodeRef: nodeRef,
			Kind:    kind,
		})
	}
	return out
}

func normalizedSections(sections []TaskProjectionSection) []TaskProjectionSection {
	out := make([]TaskProjectionSection, 0, len(sections))
	for _, section := range sections {
		body := normalizedOrderedStrings(section.Body)
		key := strings.TrimSpace(section.Key)
		title := strings.TrimSpace(section.Title)
		if key == "" && title == "" && len(body) == 0 {
			continue
		}
		out = append(out, TaskProjectionSection{
			Key:   key,
			Title: title,
			Body:  body,
		})
	}
	return out
}

// RuntimeFields are the tracker-owned mutable overlays pulled safely from remote state.
type RuntimeFields struct {
	Status   string
	Assignee string
	Priority string
}

// PushResult captures idempotent push behavior.
type PushResult struct {
	RemoteID   string
	Mutated    bool
	Noop       bool
	Diagnostic string
}

// TrackerAdapter defines the base adapter contract.
// - push_task: push canonical plan-derived task projection to tracker.
// - pull_runtime_fields: pull tracker-owned runtime overlays only.
type TrackerAdapter interface {
	PushTask(ctx context.Context, task TaskProjection) (PushResult, error)
	PullRuntimeFields(ctx context.Context, ids []string) (map[string]RuntimeFields, error)
}
