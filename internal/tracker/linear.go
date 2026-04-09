package tracker

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/cache"
	"github.com/vikramoddiraju/planmark/internal/fsio"
)

const LinearRenderProfile = RenderProfileDefault

type LinearIssuePayload struct {
	ProjectionSchemaVersion string   `json:"projection_schema_version"`
	ID                      string   `json:"id"`
	Title                   string   `json:"title"`
	Description             string   `json:"description,omitempty"`
	Labels                  []string `json:"labels,omitempty"`
}

type LinearAdapter struct {
	renderProfile      RenderProfile
	projectionHashByID map[string]string
	remoteIDByID       map[string]string
	provenanceByID     map[string]TaskProvenance
	runtimeByID        map[string]RuntimeFields
	lastSeenRuntime    map[string]string
}

func NewLinearAdapter() *LinearAdapter {
	return &LinearAdapter{
		renderProfile:      LinearRenderProfile,
		projectionHashByID: map[string]string{},
		remoteIDByID:       map[string]string{},
		provenanceByID:     map[string]TaskProvenance{},
		runtimeByID:        map[string]RuntimeFields{},
		lastSeenRuntime:    map[string]string{},
	}
}

func (a *LinearAdapter) SetRenderProfile(profile RenderProfile) {
	a.renderProfile = normalizeRenderProfile(profile)
	if a.renderProfile == "" {
		a.renderProfile = LinearRenderProfile
	}
}

func (a *LinearAdapter) Capabilities() TrackerCapabilities {
	return TrackerCapabilities{
		AdapterName:  "linear",
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
		ProjectionSchema: ProjectionSchemaVersionV03,
	}
}

func BuildLinearIssuePayload(task TaskProjection) (LinearIssuePayload, error) {
	rendered, err := RenderTask(task, NewLinearAdapter().Capabilities(), LinearRenderProfile)
	if err != nil {
		return LinearIssuePayload{}, fmt.Errorf("render linear issue: %w", err)
	}
	return buildLinearIssuePayloadFromRendered(task, rendered)
}

func (a *LinearAdapter) RenderTaskProjection(task TaskProjection) (RenderedTask, error) {
	profile := a.renderProfile
	if profile == "" {
		profile = LinearRenderProfile
	}
	return RenderTask(task, a.Capabilities(), profile)
}

func (a *LinearAdapter) ValidateTask(task TaskProjection) error {
	_, err := a.RenderTaskProjection(task)
	return err
}

func buildLinearIssuePayloadFromRendered(task TaskProjection, rendered RenderedTask) (LinearIssuePayload, error) {
	id := strings.TrimSpace(task.ID)
	payload := LinearIssuePayload{
		ProjectionSchemaVersion: normalizedProjectionVersion(task.ProjectionVersion),
		ID:                      id,
		Title:                   rendered.Title,
		Description:             joinRenderedBody(rendered.Body),
		Labels:                  linearLabels(task),
	}
	if payload.ID == "" {
		return LinearIssuePayload{}, fmt.Errorf("task projection requires non-empty id")
	}
	return payload, nil
}

func (a *LinearAdapter) PushTask(_ context.Context, task TaskProjection) (PushResult, error) {
	rendered, err := a.RenderTaskProjection(task)
	if err != nil {
		return PushResult{}, err
	}
	if _, err := buildLinearIssuePayloadFromRendered(task, rendered); err != nil {
		return PushResult{}, err
	}
	id := strings.TrimSpace(task.ID)
	currentHash, err := TaskProjectionHash(task)
	if err != nil {
		return PushResult{}, fmt.Errorf("hash task projection: %w", err)
	}
	currentStatus := strings.TrimSpace(a.runtimeByID[id].Status)
	shouldClose := taskShouldBeClosed(task)
	if previousHash, ok := a.projectionHashByID[id]; ok && previousHash == currentHash {
		if shouldClose && !strings.EqualFold(currentStatus, "closed") {
			state := a.runtimeByID[id]
			state.Status = "closed"
			a.runtimeByID[id] = state
			return PushResult{
				RemoteID:   a.remoteIDByID[id],
				Mutated:    true,
				Noop:       false,
				Diagnostic: "closed tracker issue for canonically completed task",
			}, nil
		}
		if !shouldClose && strings.EqualFold(currentStatus, "closed") {
			state := a.runtimeByID[id]
			state.Status = "open"
			a.runtimeByID[id] = state
			return PushResult{
				RemoteID:   a.remoteIDByID[id],
				Mutated:    true,
				Noop:       false,
				Diagnostic: "reopened closed tracker issue with unchanged projection",
			}, nil
		}
		return PushResult{
			RemoteID:   a.remoteIDByID[id],
			Mutated:    false,
			Noop:       true,
			Diagnostic: "projection unchanged",
		}, nil
	}

	remoteID := a.remoteIDByID[id]
	if remoteID == "" {
		remoteID = "linear:" + id
	}
	a.projectionHashByID[id] = currentHash
	a.remoteIDByID[id] = remoteID
	a.provenanceByID[id] = normalizedProvenance(task.Provenance)
	state := a.runtimeByID[id]
	if shouldClose {
		state.Status = "closed"
	} else {
		state.Status = "open"
	}
	a.runtimeByID[id] = state

	return PushResult{
		RemoteID:   remoteID,
		Mutated:    true,
		Noop:       false,
		Diagnostic: "projection updated",
	}, nil
}

func (a *LinearAdapter) SeedFromSyncManifest(manifest SyncManifest) {
	for _, entry := range manifest.Entries {
		id := strings.TrimSpace(entry.ID)
		if id == "" {
			continue
		}
		a.remoteIDByID[id] = strings.TrimSpace(entry.RemoteID)
		a.projectionHashByID[id] = strings.TrimSpace(entry.ProjectionHash)
		a.provenanceByID[id] = TaskProvenance{
			NodeRef:    strings.TrimSpace(entry.NodeRef),
			Path:       strings.TrimSpace(entry.SourcePath),
			StartLine:  entry.SourceStartLine,
			EndLine:    entry.SourceEndLine,
			SourceHash: strings.TrimSpace(entry.SourceHash),
			CompileID:  strings.TrimSpace(entry.CompileID),
		}
		a.lastSeenRuntime[id] = strings.TrimSpace(entry.LastSeenRuntimeHash)
	}
}

func (a *LinearAdapter) PullRuntimeFields(_ context.Context, ids []string) (map[string]RuntimeFields, error) {
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

func (a *LinearAdapter) SetRemoteRuntimeFields(id string, fields RuntimeFields) {
	a.runtimeByID[strings.TrimSpace(id)] = fields
}

func (a *LinearAdapter) BuildSyncManifest() SyncManifest {
	idsSet := map[string]struct{}{}
	for id := range a.projectionHashByID {
		idsSet[id] = struct{}{}
	}
	for id := range a.remoteIDByID {
		idsSet[id] = struct{}{}
	}
	for id := range a.lastSeenRuntime {
		idsSet[id] = struct{}{}
	}
	ids := make([]string, 0, len(idsSet))
	for id := range idsSet {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	entries := make([]SyncManifestEntry, 0, len(ids))
	for _, id := range ids {
		entries = append(entries, SyncManifestEntry{
			ID:                  id,
			RemoteID:            a.remoteIDByID[id],
			ProjectionHash:      a.projectionHashByID[id],
			NodeRef:             a.provenanceByID[id].NodeRef,
			SourcePath:          a.provenanceByID[id].Path,
			SourceStartLine:     a.provenanceByID[id].StartLine,
			SourceEndLine:       a.provenanceByID[id].EndLine,
			SourceHash:          a.provenanceByID[id].SourceHash,
			CompileID:           a.provenanceByID[id].CompileID,
			LastSeenRuntimeHash: a.lastSeenRuntime[id],
		})
	}
	return SyncManifest{
		SchemaVersion: SyncManifestSchemaVersionV01,
		Entries:       entries,
	}
}

func (a *LinearAdapter) WriteSyncManifest(stateDir string) (string, error) {
	resolvedStateDir := strings.TrimSpace(stateDir)
	if resolvedStateDir == "" {
		resolvedStateDir = ".planmark"
	}
	lock, err := cache.AcquireLock(resolvedStateDir, "sync-linear-manifest")
	if err != nil {
		return "", fmt.Errorf("acquire sync manifest lock: %w", err)
	}
	defer func() {
		_ = lock.Release()
	}()

	manifestPath := filepath.Join(resolvedStateDir, "sync", "linear-manifest.json")
	manifest := a.BuildSyncManifest()
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal sync manifest: %w", err)
	}
	data = append(data, '\n')
	if err := fsio.WriteFileAtomic(manifestPath, data, 0o644); err != nil {
		return "", fmt.Errorf("write sync manifest: %w", err)
	}
	if err := lock.Release(); err != nil {
		return "", fmt.Errorf("release sync manifest lock: %w", err)
	}
	lock = nil
	return manifestPath, nil
}

func linearLabels(task TaskProjection) []string {
	labels := make([]string, 0, 2)
	if horizon := strings.TrimSpace(task.Horizon); horizon != "" {
		labels = append(labels, "horizon:"+horizon)
	}
	return labels
}
