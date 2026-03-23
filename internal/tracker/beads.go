package tracker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/cache"
	"github.com/vikramoddiraju/planmark/internal/fsio"
)

const ProjectionSchemaVersionV02 = "v0.2"
const BeadsManifestSchemaVersionV01 = "v0.1"

var ErrTransientSync = errors.New("transient tracker sync error")
var ErrRateLimitedSync = errors.New("tracker rate limited")

type SourceRange struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

type BeadsProjectionPayload struct {
	ProjectionSchemaVersion string      `json:"projection_schema_version"`
	ID                      string      `json:"id"`
	Title                   string      `json:"title"`
	Horizon                 string      `json:"horizon,omitempty"`
	Anchor                  string      `json:"anchor"`
	SourceRange             SourceRange `json:"source_range"`
	SourceHash              string      `json:"source_hash"`
	Dependencies            []string    `json:"dependencies,omitempty"`
	AcceptanceDigest        string      `json:"acceptance_digest"`
	Steps                   []BeadsStep `json:"steps,omitempty"`
	EvidenceNodeRefs        []string    `json:"evidence_node_refs,omitempty"`
}

type BeadsStep struct {
	Title   string `json:"title"`
	Checked bool   `json:"checked,omitempty"`
	NodeRef string `json:"node_ref,omitempty"`
}

type BeadsAdapter struct {
	projectionHashByID map[string]string
	sourceHashByID     map[string]string
	provenanceByID     map[string]TaskProvenance
	remoteIDByID       map[string]string
	runtimeByID        map[string]RuntimeFields
	lastSeenRuntime    map[string]string
	pushFailuresByID   map[string][]error
}

type BeadsManifestEntry struct {
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

type BeadsSyncManifest struct {
	SchemaVersion string               `json:"schema_version"`
	Entries       []BeadsManifestEntry `json:"entries"`
}

func NewBeadsAdapter() *BeadsAdapter {
	return &BeadsAdapter{
		projectionHashByID: map[string]string{},
		sourceHashByID:     map[string]string{},
		provenanceByID:     map[string]TaskProvenance{},
		remoteIDByID:       map[string]string{},
		runtimeByID:        map[string]RuntimeFields{},
		lastSeenRuntime:    map[string]string{},
		pushFailuresByID:   map[string][]error{},
	}
}

func (a *BeadsAdapter) SeedFromSyncManifest(manifest BeadsSyncManifest) {
	for _, entry := range manifest.Entries {
		id := strings.TrimSpace(entry.ID)
		if id == "" {
			continue
		}
		a.remoteIDByID[id] = strings.TrimSpace(entry.RemoteID)
		a.projectionHashByID[id] = strings.TrimSpace(entry.ProjectionHash)
		a.sourceHashByID[id] = strings.TrimSpace(entry.SourceHash)
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

func BuildProjectionPayload(task TaskProjection) (BeadsProjectionPayload, error) {
	if strings.TrimSpace(task.ID) == "" {
		return BeadsProjectionPayload{}, fmt.Errorf("task projection requires non-empty id")
	}
	if strings.TrimSpace(task.SourcePath) == "" {
		return BeadsProjectionPayload{}, fmt.Errorf("task projection %q requires source path", task.ID)
	}
	if task.SourceStartLine <= 0 || task.SourceEndLine < task.SourceStartLine {
		return BeadsProjectionPayload{}, fmt.Errorf("task projection %q has invalid source range %d-%d", task.ID, task.SourceStartLine, task.SourceEndLine)
	}
	if strings.TrimSpace(task.SourceHash) == "" {
		return BeadsProjectionPayload{}, fmt.Errorf("task projection %q requires source hash", task.ID)
	}

	anchor := strings.TrimSpace(task.Anchor)
	if anchor == "" {
		anchor = fmt.Sprintf("%s#L%d", task.SourcePath, task.SourceStartLine)
	}
	projectionVersion := strings.TrimSpace(task.ProjectionVersion)
	if projectionVersion == "" {
		projectionVersion = ProjectionSchemaVersionV02
	}

	return BeadsProjectionPayload{
		ProjectionSchemaVersion: projectionVersion,
		ID:                      task.ID,
		Title:                   task.Title,
		Horizon:                 strings.TrimSpace(task.Horizon),
		Anchor:                  anchor,
		SourceRange: SourceRange{
			Path:      task.SourcePath,
			StartLine: task.SourceStartLine,
			EndLine:   task.SourceEndLine,
		},
		SourceHash:       task.SourceHash,
		Dependencies:     orderedStrings(task.Deps),
		AcceptanceDigest: acceptanceDigest(task.Accept),
		Steps:            buildBeadsSteps(task.Steps),
		EvidenceNodeRefs: orderedStrings(task.EvidenceNodeRefs),
	}, nil
}

func (a *BeadsAdapter) PushTask(_ context.Context, task TaskProjection) (PushResult, error) {
	if queued := a.pushFailuresByID[task.ID]; len(queued) > 0 {
		err := queued[0]
		if len(queued) == 1 {
			delete(a.pushFailuresByID, task.ID)
		} else {
			a.pushFailuresByID[task.ID] = queued[1:]
		}
		return PushResult{}, err
	}

	payload, err := BuildProjectionPayload(task)
	if err != nil {
		return PushResult{}, err
	}
	drifted, err := a.DetectProjectionDrift(task)
	if err != nil {
		return PushResult{}, err
	}
	currentHash, err := projectionHash(payload)
	if err != nil {
		return PushResult{}, err
	}

	previousHash, hasPrevious := a.projectionHashByID[task.ID]
	if hasPrevious && previousHash == currentHash {
		return PushResult{
			RemoteID:   a.remoteIDByID[task.ID],
			Mutated:    false,
			Noop:       true,
			Diagnostic: "projection unchanged",
		}, nil
	}

	remoteID := a.remoteIDByID[task.ID]
	if remoteID == "" {
		remoteID = "beads:" + task.ID
	}
	a.projectionHashByID[task.ID] = currentHash
	a.sourceHashByID[task.ID] = payload.SourceHash
	a.provenanceByID[task.ID] = TaskProvenance{
		NodeRef:    strings.TrimSpace(task.NodeRef),
		Path:       task.SourcePath,
		StartLine:  task.SourceStartLine,
		EndLine:    task.SourceEndLine,
		SourceHash: task.SourceHash,
		CompileID:  strings.TrimSpace(task.CompileID),
	}
	a.remoteIDByID[task.ID] = remoteID

	diagnostic := "projection updated"
	if drifted {
		diagnostic = "projection drift detected: source hash changed"
	}

	return PushResult{
		RemoteID:   remoteID,
		Mutated:    true,
		Noop:       false,
		Diagnostic: diagnostic,
	}, nil
}

func (a *BeadsAdapter) DetectProjectionDrift(task TaskProjection) (bool, error) {
	if strings.TrimSpace(task.ID) == "" {
		return false, fmt.Errorf("task projection requires non-empty id")
	}
	if strings.TrimSpace(task.SourceHash) == "" {
		return false, fmt.Errorf("task projection %q requires source hash", task.ID)
	}

	previousSourceHash, hasPrevious := a.sourceHashByID[task.ID]
	if !hasPrevious {
		return false, nil
	}
	return previousSourceHash != task.SourceHash, nil
}

func (a *BeadsAdapter) PullRuntimeFields(_ context.Context, ids []string) (map[string]RuntimeFields, error) {
	out := make(map[string]RuntimeFields, len(ids))
	for _, id := range ids {
		state, ok := a.runtimeByID[id]
		if !ok {
			continue
		}

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

func (a *BeadsAdapter) SetRemoteRuntimeFields(id string, fields RuntimeFields) {
	a.runtimeByID[id] = fields
}

func (a *BeadsAdapter) SetPushFailures(id string, failures []error) {
	if len(failures) == 0 {
		delete(a.pushFailuresByID, id)
		return
	}
	copied := make([]error, len(failures))
	copy(copied, failures)
	a.pushFailuresByID[id] = copied
}

func IsTransientSyncError(err error) bool {
	return errors.Is(err, ErrTransientSync)
}

func IsRateLimitedSyncError(err error) bool {
	return errors.Is(err, ErrRateLimitedSync)
}

func IsRetryableSyncError(err error) bool {
	return IsTransientSyncError(err) || IsRateLimitedSyncError(err)
}

func (a *BeadsAdapter) BuildSyncManifest() BeadsSyncManifest {
	idsSet := map[string]struct{}{}
	for id := range a.projectionHashByID {
		idsSet[id] = struct{}{}
	}
	for id := range a.sourceHashByID {
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

	entries := make([]BeadsManifestEntry, 0, len(ids))
	for _, id := range ids {
		entries = append(entries, BeadsManifestEntry{
			ID:                  id,
			RemoteID:            a.remoteIDByID[id],
			ProjectionHash:      a.projectionHashByID[id],
			NodeRef:             a.provenanceByID[id].NodeRef,
			SourcePath:          a.provenanceByID[id].Path,
			SourceStartLine:     a.provenanceByID[id].StartLine,
			SourceEndLine:       a.provenanceByID[id].EndLine,
			SourceHash:          a.sourceHashByID[id],
			CompileID:           a.provenanceByID[id].CompileID,
			LastSeenRuntimeHash: a.lastSeenRuntime[id],
		})
	}

	return BeadsSyncManifest{
		SchemaVersion: BeadsManifestSchemaVersionV01,
		Entries:       entries,
	}
}

func (a *BeadsAdapter) WriteSyncManifest(stateDir string) (string, error) {
	resolvedStateDir := strings.TrimSpace(stateDir)
	if resolvedStateDir == "" {
		resolvedStateDir = ".planmark"
	}
	lock, err := cache.AcquireLock(resolvedStateDir, "sync-beads-manifest")
	if err != nil {
		return "", fmt.Errorf("acquire sync manifest lock: %w", err)
	}
	defer func() {
		_ = lock.Release()
	}()

	manifestPath := filepath.Join(resolvedStateDir, "sync", "beads-manifest.json")
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

func acceptanceDigest(values []string) string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	sum := sha256.Sum256([]byte(strings.Join(normalized, "\n")))
	return hex.EncodeToString(sum[:])
}

func projectionHash(payload BeadsProjectionPayload) (string, error) {
	blob, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal projection payload: %w", err)
	}
	sum := sha256.Sum256(blob)
	return hex.EncodeToString(sum[:]), nil
}

func buildBeadsSteps(steps []TaskProjectionStep) []BeadsStep {
	projected := make([]BeadsStep, 0, len(steps))
	for _, step := range steps {
		title := strings.TrimSpace(step.Title)
		if title == "" {
			continue
		}
		projected = append(projected, BeadsStep{
			Title:   title,
			Checked: step.Checked,
			NodeRef: strings.TrimSpace(step.NodeRef),
		})
	}
	return projected
}

func orderedStrings(values []string) []string {
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

func runtimeHash(fields RuntimeFields) (string, error) {
	blob, err := json.Marshal(fields)
	if err != nil {
		return "", fmt.Errorf("marshal runtime fields: %w", err)
	}
	sum := sha256.Sum256(blob)
	return hex.EncodeToString(sum[:]), nil
}
