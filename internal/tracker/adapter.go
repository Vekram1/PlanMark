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
	CanonicalStatus   string
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

type CapabilitySupport string

const (
	CapabilityUnsupported CapabilitySupport = "unsupported"
	CapabilityRendered    CapabilitySupport = "rendered"
	CapabilityNative      CapabilitySupport = "native"
)

type TextCapability string

const (
	TextUnsupported TextCapability = "unsupported"
	TextPlain       TextCapability = "plain"
	TextMarkdown    TextCapability = "markdown"
)

type RuntimeOverlayCapabilities struct {
	Status   bool `json:"status"`
	Assignee bool `json:"assignee"`
	Priority bool `json:"priority"`
}

// TrackerCapabilities declares the deterministic shape a tracker adapter can
// currently represent. Renderers/templates use this to validate whether a
// projection can be expressed directly, must be collapsed into body text, or is
// unsupported by the backend.
type TrackerCapabilities struct {
	AdapterName      string                     `json:"adapter_name"`
	Title            bool                       `json:"title"`
	Body             TextCapability             `json:"body"`
	Steps            CapabilitySupport          `json:"steps"`
	ChildWork        CapabilitySupport          `json:"child_work"`
	CustomFields     CapabilitySupport          `json:"custom_fields"`
	RuntimeOverlays  RuntimeOverlayCapabilities `json:"runtime_overlays"`
	ProjectionSchema string                     `json:"projection_schema,omitempty"`
}

type canonicalTaskProjection struct {
	ID                string                   `json:"id"`
	Title             string                   `json:"title,omitempty"`
	CanonicalStatus   string                   `json:"canonical_status"`
	Horizon           string                   `json:"horizon,omitempty"`
	Anchor            string                   `json:"anchor,omitempty"`
	Dependencies      []string                 `json:"dependencies,omitempty"`
	Acceptance        []string                 `json:"acceptance,omitempty"`
	Steps             []TaskProjectionStep     `json:"steps,omitempty"`
	Evidence          []TaskProjectionEvidence `json:"evidence,omitempty"`
	Sections          []TaskProjectionSection  `json:"sections,omitempty"`
	ProjectionVersion string                   `json:"projection_version,omitempty"`
}

// TaskProjectionHash computes a deterministic hash over the semantic
// tracker-neutral task projection. Provenance is intentionally excluded so
// source line churn or slice re-addressing does not force broad tracker updates
// when the task's rendered meaning is unchanged.
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
		CanonicalStatus:   normalizeCanonicalTaskStatus(task.CanonicalStatus),
		Horizon:           strings.TrimSpace(task.Horizon),
		Anchor:            strings.TrimSpace(task.Anchor),
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
		return ProjectionSchemaVersionV03
	}
	return trimmed
}

func normalizeCanonicalTaskStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "done":
		return "done"
	default:
		return "open"
	}
}

func taskShouldBeClosed(task TaskProjection) bool {
	return normalizeCanonicalTaskStatus(task.CanonicalStatus) == "done"
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
		body := normalizedSectionBody(section.Body)
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

func normalizedSectionBody(lines []string) []string {
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	out := make([]string, 0, end-start)
	lastBlank := false
	for _, line := range lines[start:end] {
		normalized := strings.TrimRight(line, " \t")
		isBlank := strings.TrimSpace(normalized) == ""
		if isBlank {
			if lastBlank {
				continue
			}
			lastBlank = true
			out = append(out, "")
			continue
		}
		lastBlank = false
		out = append(out, normalized)
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
	Capabilities() TrackerCapabilities
}

const SyncManifestSchemaVersionV01 = "v0.1"

type SyncManifestEntry struct {
	ID                  string `json:"id"`
	RemoteID            string `json:"remote_id,omitempty"`
	ProjectionHash      string `json:"projection_hash,omitempty"`
	NodeRef             string `json:"node_ref,omitempty"`
	SourcePath          string `json:"source_path,omitempty"`
	SourceStartLine     int    `json:"source_start_line,omitempty"`
	SourceEndLine       int    `json:"source_end_line,omitempty"`
	SourceHash          string `json:"source_hash,omitempty"`
	CompileID           string `json:"compile_id,omitempty"`
	LastSeenRuntimeHash string `json:"last_seen_runtime_hash,omitempty"`
}

type SyncManifest struct {
	SchemaVersion string              `json:"schema_version"`
	Entries       []SyncManifestEntry `json:"entries"`
}

type BeadsManifestEntry = SyncManifestEntry
type BeadsSyncManifest = SyncManifest

type CleanupCandidate struct {
	RemoteID    string `json:"remote_id"`
	Title       string `json:"title,omitempty"`
	ExternalRef string `json:"external_ref,omitempty"`
	SourcePath  string `json:"source_path,omitempty"`
	Reason      string `json:"reason"`
}
