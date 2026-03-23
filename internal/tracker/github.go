package tracker

import (
	"context"
	"fmt"
	"strings"
)

const GitHubRenderProfile = RenderProfileDefault

type GitHubIssuePayload struct {
	ProjectionSchemaVersion string   `json:"projection_schema_version"`
	ID                      string   `json:"id"`
	Title                   string   `json:"title"`
	Body                    string   `json:"body,omitempty"`
	Labels                  []string `json:"labels,omitempty"`
}

type GitHubAdapter struct {
	projectionHashByID map[string]string
	remoteIDByID       map[string]string
	runtimeByID        map[string]RuntimeFields
	lastSeenRuntime    map[string]string
}

func NewGitHubAdapter() *GitHubAdapter {
	return &GitHubAdapter{
		projectionHashByID: map[string]string{},
		remoteIDByID:       map[string]string{},
		runtimeByID:        map[string]RuntimeFields{},
		lastSeenRuntime:    map[string]string{},
	}
}

func (a *GitHubAdapter) Capabilities() TrackerCapabilities {
	return TrackerCapabilities{
		AdapterName:  "github",
		Title:        true,
		Body:         TextMarkdown,
		Steps:        CapabilityRendered,
		ChildWork:    CapabilityUnsupported,
		CustomFields: CapabilityUnsupported,
		RuntimeOverlays: RuntimeOverlayCapabilities{
			Status:   true,
			Assignee: true,
			Priority: false,
		},
		ProjectionSchema: ProjectionSchemaVersionV02,
	}
}

func BuildGitHubIssuePayload(task TaskProjection) (GitHubIssuePayload, error) {
	rendered, err := RenderTask(task, NewGitHubAdapter().Capabilities(), GitHubRenderProfile)
	if err != nil {
		return GitHubIssuePayload{}, fmt.Errorf("render github issue: %w", err)
	}
	id := strings.TrimSpace(task.ID)
	payload := GitHubIssuePayload{
		ProjectionSchemaVersion: normalizedProjectionVersion(task.ProjectionVersion),
		ID:                      id,
		Title:                   rendered.Title,
		Body:                    joinRenderedBody(rendered.Body),
		Labels:                  githubLabels(task),
	}
	if payload.ID == "" {
		return GitHubIssuePayload{}, fmt.Errorf("task projection requires non-empty id")
	}
	return payload, nil
}

func (a *GitHubAdapter) PushTask(_ context.Context, task TaskProjection) (PushResult, error) {
	if _, err := BuildGitHubIssuePayload(task); err != nil {
		return PushResult{}, err
	}
	id := strings.TrimSpace(task.ID)
	currentHash, err := TaskProjectionHash(task)
	if err != nil {
		return PushResult{}, fmt.Errorf("hash task projection: %w", err)
	}
	if previousHash, ok := a.projectionHashByID[id]; ok && previousHash == currentHash {
		return PushResult{
			RemoteID:   a.remoteIDByID[id],
			Mutated:    false,
			Noop:       true,
			Diagnostic: "projection unchanged",
		}, nil
	}

	remoteID := a.remoteIDByID[id]
	if remoteID == "" {
		remoteID = "github:" + id
	}
	a.projectionHashByID[id] = currentHash
	a.remoteIDByID[id] = remoteID

	return PushResult{
		RemoteID:   remoteID,
		Mutated:    true,
		Noop:       false,
		Diagnostic: "projection updated",
	}, nil
}

func (a *GitHubAdapter) PullRuntimeFields(_ context.Context, ids []string) (map[string]RuntimeFields, error) {
	out := make(map[string]RuntimeFields, len(ids))
	for _, rawID := range ids {
		id := strings.TrimSpace(rawID)
		state, ok := a.runtimeByID[id]
		if !ok {
			continue
		}
		state.Priority = ""
		hash, err := runtimeHash(state)
		if err != nil {
			return nil, err
		}
		if prev, seen := a.lastSeenRuntime[id]; seen && prev == hash {
			continue
		}
		a.lastSeenRuntime[id] = hash
		out[id] = state
	}
	return out, nil
}

func (a *GitHubAdapter) SetRemoteRuntimeFields(id string, fields RuntimeFields) {
	a.runtimeByID[strings.TrimSpace(id)] = fields
}

func joinRenderedBody(lines []string) string {
	lines = trimBlankEdges(lines)
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func githubLabels(task TaskProjection) []string {
	labels := make([]string, 0, 2)
	if horizon := strings.TrimSpace(task.Horizon); horizon != "" {
		labels = append(labels, "horizon:"+horizon)
	}
	return labels
}
