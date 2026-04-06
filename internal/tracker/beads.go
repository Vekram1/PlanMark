package tracker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/cache"
	"github.com/vikramoddiraju/planmark/internal/fsio"
)

const ProjectionSchemaVersionV02 = "v0.2"
const BeadsManifestSchemaVersionV01 = SyncManifestSchemaVersionV01
const BeadsRenderProfile = RenderProfileDefault

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
	renderProfile      RenderProfile
	projectionHashByID map[string]string
	sourceHashByID     map[string]string
	provenanceByID     map[string]TaskProvenance
	remoteIDByID       map[string]string
	runtimeByID        map[string]RuntimeFields
	lastSeenRuntime    map[string]string
	pushFailuresByID   map[string][]error
}

var runBrCommand = func(args ...string) ([]byte, error) {
	cmd := exec.Command("br", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if len(output) == 0 {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

type beadsIssue struct {
	ID                 string `json:"id"`
	Title              string `json:"title"`
	Status             string `json:"status,omitempty"`
	Assignee           string `json:"assignee,omitempty"`
	ExternalRef        string `json:"external_ref,omitempty"`
	Description        string `json:"description,omitempty"`
	AcceptanceCriteria string `json:"acceptance_criteria,omitempty"`
}

type beadsErrorEnvelope struct {
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
}

func NewBeadsAdapter() *BeadsAdapter {
	return &BeadsAdapter{
		renderProfile:      BeadsRenderProfile,
		projectionHashByID: map[string]string{},
		sourceHashByID:     map[string]string{},
		provenanceByID:     map[string]TaskProvenance{},
		remoteIDByID:       map[string]string{},
		runtimeByID:        map[string]RuntimeFields{},
		lastSeenRuntime:    map[string]string{},
		pushFailuresByID:   map[string][]error{},
	}
}

func (a *BeadsAdapter) SetRenderProfile(profile RenderProfile) {
	a.renderProfile = normalizeRenderProfile(profile)
	if a.renderProfile == "" {
		a.renderProfile = BeadsRenderProfile
	}
}

func (a *BeadsAdapter) Capabilities() TrackerCapabilities {
	return TrackerCapabilities{
		AdapterName:  "beads",
		Title:        true,
		Body:         TextMarkdown,
		Steps:        CapabilityRendered,
		ChildWork:    CapabilityUnsupported,
		CustomFields: CapabilityUnsupported,
		RuntimeOverlays: RuntimeOverlayCapabilities{
			Status:   true,
			Assignee: true,
			Priority: true,
		},
		ProjectionSchema: ProjectionSchemaVersionV02,
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

func (a *BeadsAdapter) ReconcileSyncManifest(_ context.Context, manifest BeadsSyncManifest) (BeadsSyncManifest, error) {
	reconciled := BeadsSyncManifest{
		SchemaVersion: manifest.SchemaVersion,
		Entries:       make([]BeadsManifestEntry, 0, len(manifest.Entries)),
	}
	if reconciled.SchemaVersion == "" {
		reconciled.SchemaVersion = BeadsManifestSchemaVersionV01
	}
	for _, entry := range manifest.Entries {
		if !isLikelyBeadsIssueID(entry.RemoteID) {
			continue
		}
		if _, err := a.lookupIssue(entry.RemoteID); err != nil {
			if isBeadsIssueNotFound(err) {
				continue
			}
			return BeadsSyncManifest{}, err
		}
		reconciled.Entries = append(reconciled.Entries, entry)
	}
	return reconciled, nil
}

func BuildProjectionPayload(task TaskProjection) (BeadsProjectionPayload, error) {
	rendered, err := RenderTask(task, NewBeadsAdapter().Capabilities(), BeadsRenderProfile)
	if err != nil {
		return BeadsProjectionPayload{}, fmt.Errorf("render beads task: %w", err)
	}
	return buildProjectionPayloadFromRendered(task, rendered)
}

func (a *BeadsAdapter) RenderTaskProjection(task TaskProjection) (RenderedTask, error) {
	profile := a.renderProfile
	if profile == "" {
		profile = BeadsRenderProfile
	}
	return RenderTask(task, a.Capabilities(), profile)
}

func (a *BeadsAdapter) ValidateTask(task TaskProjection) error {
	_, err := a.RenderTaskProjection(task)
	return err
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

	rendered, err := a.RenderTaskProjection(task)
	if err != nil {
		return PushResult{}, err
	}
	payload, err := buildProjectionPayloadFromRendered(task, rendered)
	if err != nil {
		return PushResult{}, err
	}
	drifted, err := a.DetectProjectionDrift(task)
	if err != nil {
		return PushResult{}, err
	}
	currentHash, err := TaskProjectionHash(task)
	if err != nil {
		return PushResult{}, fmt.Errorf("hash task projection: %w", err)
	}

	previousHash, hasPrevious := a.projectionHashByID[task.ID]
	if hasPrevious && previousHash == currentHash && isLikelyBeadsIssueID(a.remoteIDByID[task.ID]) {
		return PushResult{
			RemoteID:   a.remoteIDByID[task.ID],
			Mutated:    false,
			Noop:       true,
			Diagnostic: "projection unchanged",
		}, nil
	}

	remoteID := strings.TrimSpace(a.remoteIDByID[task.ID])
	description := strings.Join(rendered.Body, "\n")
	acceptance := strings.Join(orderedStrings(task.Acceptance), "\n")
	if !isLikelyBeadsIssueID(remoteID) {
		existing, err := a.lookupIssueByExternalRef(task.ID)
		if err != nil {
			return PushResult{}, err
		}
		if isLikelyBeadsIssueID(existing.ID) {
			remoteID = existing.ID
		}
	}
	if isLikelyBeadsIssueID(remoteID) {
		if _, err := a.updateIssueWithExternalRef(remoteID, task.ID, rendered.Title, description, acceptance); err != nil {
			return PushResult{}, err
		}
	} else {
		issue, err := a.createIssueWithExternalRef(task.ID, rendered.Title, description, acceptance)
		if err != nil {
			return PushResult{}, err
		}
		remoteID = issue.ID
	}
	a.projectionHashByID[task.ID] = currentHash
	a.sourceHashByID[task.ID] = payload.SourceHash
	a.provenanceByID[task.ID] = normalizedProvenance(task.Provenance)
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

func (a *BeadsAdapter) createIssue(title string, description string, acceptance string) (beadsIssue, error) {
	return a.createIssueWithExternalRef("", title, description, acceptance)
}

func (a *BeadsAdapter) createIssueWithExternalRef(externalRef string, title string, description string, acceptance string) (beadsIssue, error) {
	args := []string{"create", "--title", title, "--type", "task", "--json"}
	if strings.TrimSpace(externalRef) != "" {
		args = append(args, "--external-ref", externalRef)
	}
	createDescription := description
	if strings.TrimSpace(acceptance) != "" {
		if strings.TrimSpace(createDescription) != "" {
			createDescription += "\n\n## Acceptance\n" + acceptance
		} else {
			createDescription = "## Acceptance\n" + acceptance
		}
	}
	if strings.TrimSpace(createDescription) != "" {
		args = append(args, "--description", createDescription)
	}
	output, err := runBrCommand(args...)
	if err != nil {
		return beadsIssue{}, fmt.Errorf("br create: %w", err)
	}
	var issue beadsIssue
	if err := json.Unmarshal(output, &issue); err != nil {
		return beadsIssue{}, fmt.Errorf("decode br create output: %w", err)
	}
	if !isLikelyBeadsIssueID(issue.ID) {
		return beadsIssue{}, fmt.Errorf("br create returned invalid issue id %q", issue.ID)
	}
	return issue, nil
}

func (a *BeadsAdapter) updateIssue(id string, title string, description string, acceptance string) (beadsIssue, error) {
	return a.updateIssueWithExternalRef(id, "", title, description, acceptance)
}

func (a *BeadsAdapter) updateIssueWithExternalRef(id string, externalRef string, title string, description string, acceptance string) (beadsIssue, error) {
	args := []string{"update", id, "--title", title, "--description", description, "--acceptance-criteria", acceptance}
	if strings.TrimSpace(externalRef) != "" {
		args = append(args, "--external-ref", externalRef)
	}
	args = append(args, "--json")
	output, err := runBrCommand(args...)
	if err != nil {
		return beadsIssue{}, fmt.Errorf("br update %s: %w", id, err)
	}
	var issues []beadsIssue
	if err := json.Unmarshal(output, &issues); err != nil {
		return beadsIssue{}, fmt.Errorf("decode br update output: %w", err)
	}
	if len(issues) == 0 {
		return beadsIssue{}, fmt.Errorf("br update %s returned no issues", id)
	}
	return issues[0], nil
}

func (a *BeadsAdapter) lookupIssue(id string) (beadsIssue, error) {
	output, err := runBrCommand("show", id, "--json")
	if err != nil {
		return beadsIssue{}, fmt.Errorf("br show %s: %w", id, err)
	}
	var issues []beadsIssue
	if err := json.Unmarshal(output, &issues); err != nil {
		return beadsIssue{}, fmt.Errorf("decode br show output: %w", err)
	}
	if len(issues) == 0 {
		return beadsIssue{}, fmt.Errorf("br show %s returned no issues", id)
	}
	return issues[0], nil
}

func (a *BeadsAdapter) lookupIssueByExternalRef(externalRef string) (beadsIssue, error) {
	if strings.TrimSpace(externalRef) == "" {
		return beadsIssue{}, nil
	}
	output, err := runBrCommand("list", "--all", "--json")
	if err != nil {
		return beadsIssue{}, fmt.Errorf("br list: %w", err)
	}
	var issues []beadsIssue
	if err := json.Unmarshal(output, &issues); err != nil {
		return beadsIssue{}, fmt.Errorf("decode br list output: %w", err)
	}
	for _, issue := range issues {
		if strings.TrimSpace(issue.ExternalRef) == strings.TrimSpace(externalRef) {
			return issue, nil
		}
	}
	return beadsIssue{}, nil
}

func isLikelyBeadsIssueID(id string) bool {
	return strings.HasPrefix(strings.TrimSpace(id), "bead-")
}

func isBeadsIssueNotFound(err error) bool {
	if err == nil {
		return false
	}
	raw := err.Error()
	if idx := strings.Index(raw, "{"); idx >= 0 {
		raw = raw[idx:]
	}
	var payload beadsErrorEnvelope
	if jsonErr := json.Unmarshal([]byte(raw), &payload); jsonErr == nil {
		return strings.EqualFold(strings.TrimSpace(payload.Error.Code), "ISSUE_NOT_FOUND")
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "issue_not_found") || strings.Contains(lower, "issue not found")
}

func buildProjectionPayloadFromRendered(task TaskProjection, rendered RenderedTask) (BeadsProjectionPayload, error) {
	if strings.TrimSpace(task.ID) == "" {
		return BeadsProjectionPayload{}, fmt.Errorf("task projection requires non-empty id")
	}
	if strings.TrimSpace(task.Provenance.Path) == "" {
		return BeadsProjectionPayload{}, fmt.Errorf("task projection %q requires source path", task.ID)
	}
	if task.Provenance.StartLine <= 0 || task.Provenance.EndLine < task.Provenance.StartLine {
		return BeadsProjectionPayload{}, fmt.Errorf("task projection %q has invalid source range %d-%d", task.ID, task.Provenance.StartLine, task.Provenance.EndLine)
	}
	if strings.TrimSpace(task.Provenance.SourceHash) == "" {
		return BeadsProjectionPayload{}, fmt.Errorf("task projection %q requires source hash", task.ID)
	}
	anchor := strings.TrimSpace(task.Anchor)
	if anchor == "" {
		anchor = fmt.Sprintf("%s#L%d", task.Provenance.Path, task.Provenance.StartLine)
	}
	projectionVersion := strings.TrimSpace(task.ProjectionVersion)
	if projectionVersion == "" {
		projectionVersion = ProjectionSchemaVersionV02
	}

	return BeadsProjectionPayload{
		ProjectionSchemaVersion: projectionVersion,
		ID:                      task.ID,
		Title:                   rendered.Title,
		Horizon:                 strings.TrimSpace(task.Horizon),
		Anchor:                  anchor,
		SourceRange: SourceRange{
			Path:      task.Provenance.Path,
			StartLine: task.Provenance.StartLine,
			EndLine:   task.Provenance.EndLine,
		},
		SourceHash:       task.Provenance.SourceHash,
		Dependencies:     orderedStrings(task.Dependencies),
		AcceptanceDigest: acceptanceDigest(task.Acceptance),
		Steps:            buildBeadsStepsFromRendered(rendered, task.Steps),
		EvidenceNodeRefs: orderedEvidenceRefs(task.Evidence),
	}, nil
}

func (a *BeadsAdapter) DetectProjectionDrift(task TaskProjection) (bool, error) {
	if strings.TrimSpace(task.ID) == "" {
		return false, fmt.Errorf("task projection requires non-empty id")
	}
	if strings.TrimSpace(task.Provenance.SourceHash) == "" {
		return false, fmt.Errorf("task projection %q requires source hash", task.ID)
	}

	previousSourceHash, hasPrevious := a.sourceHashByID[task.ID]
	if !hasPrevious {
		return false, nil
	}
	return previousSourceHash != strings.TrimSpace(task.Provenance.SourceHash), nil
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

func buildBeadsStepsFromRendered(rendered RenderedTask, fallback []TaskProjectionStep) []BeadsStep {
	if rendered.StepMode == CapabilityNative && len(rendered.Steps) > 0 {
		projected := make([]BeadsStep, 0, len(rendered.Steps))
		for _, step := range rendered.Steps {
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
	return buildBeadsSteps(fallback)
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

func orderedEvidenceRefs(evidence []TaskProjectionEvidence) []string {
	ordered := make([]string, 0, len(evidence))
	for _, item := range evidence {
		ref := strings.TrimSpace(item.NodeRef)
		if ref == "" {
			continue
		}
		ordered = append(ordered, ref)
	}
	return ordered
}

func normalizedProvenance(p TaskProvenance) TaskProvenance {
	return TaskProvenance{
		NodeRef:    strings.TrimSpace(p.NodeRef),
		Path:       strings.TrimSpace(p.Path),
		StartLine:  p.StartLine,
		EndLine:    p.EndLine,
		SourceHash: strings.TrimSpace(p.SourceHash),
		CompileID:  strings.TrimSpace(p.CompileID),
	}
}

func runtimeHash(fields RuntimeFields) (string, error) {
	blob, err := json.Marshal(fields)
	if err != nil {
		return "", fmt.Errorf("marshal runtime fields: %w", err)
	}
	sum := sha256.Sum256(blob)
	return hex.EncodeToString(sum[:]), nil
}
