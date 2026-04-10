package tracker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/cache"
	"github.com/vikramoddiraju/planmark/internal/fsio"
)

const ProjectionSchemaVersionV03 = "v0.3"
const BeadsManifestSchemaVersionV01 = SyncManifestSchemaVersionV01
const BeadsRenderProfile = RenderProfileDefault

var ErrTransientSync = errors.New("transient tracker sync error")
var ErrRateLimitedSync = errors.New("tracker rate limited")
var beadsIssueIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*-[a-z0-9]+$`)

type SourceRange struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

type BeadsProjectionPayload struct {
	ProjectionSchemaVersion string      `json:"projection_schema_version"`
	ID                      string      `json:"id"`
	Title                   string      `json:"title"`
	CanonicalStatus         string      `json:"canonical_status"`
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
	dbPath             string
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
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		stderrText := strings.TrimSpace(stderr.String())
		if len(output) == 0 && stderrText == "" {
			return nil, err
		}
		if stderrText == "" {
			return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
		}
		if len(output) == 0 {
			return nil, fmt.Errorf("%w: %s", err, stderrText)
		}
		return nil, fmt.Errorf("%w: %s\n%s", err, strings.TrimSpace(string(output)), stderrText)
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

type beadsDependency struct {
	IssueID     string `json:"issue_id"`
	DependsOnID string `json:"depends_on_id"`
	Type        string `json:"type"`
	Title       string `json:"title,omitempty"`
	Status      string `json:"status,omitempty"`
	Priority    int    `json:"priority,omitempty"`
}

var sourceLinePattern = regexp.MustCompile(`^(?:- )?source: (.+):([0-9]+)-([0-9]+)$`)

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

func (a *BeadsAdapter) SetDBPath(path string) {
	a.dbPath = strings.TrimSpace(path)
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
		ProjectionSchema: ProjectionSchemaVersionV03,
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

	remoteID := strings.TrimSpace(a.remoteIDByID[task.ID])
	description := strings.Join(rendered.Body, "\n")
	acceptance := strings.Join(orderedStrings(task.Acceptance), "\n")
	priority := beadsPriorityForHorizon(task.Horizon)
	currentIssue := beadsIssue{}
	if isLikelyBeadsIssueID(remoteID) {
		issue, err := a.lookupIssue(remoteID)
		if err != nil && !isBeadsIssueNotFound(err) {
			return PushResult{}, err
		}
		if err == nil {
			currentIssue = issue
		} else {
			remoteID = ""
		}
	}
	if !isLikelyBeadsIssueID(remoteID) {
		existing, err := a.lookupIssueByExternalRef(task.ID)
		if err != nil {
			return PushResult{}, err
		}
		if isLikelyBeadsIssueID(existing.ID) {
			remoteID = existing.ID
			currentIssue = existing
		}
	}

	previousHash, hasPrevious := a.projectionHashByID[task.ID]
	if hasPrevious && previousHash == currentHash && isLikelyBeadsIssueID(remoteID) && !drifted {
		if shouldBeClosed(task) {
			if isBeadsIssueClosed(currentIssue) {
				return PushResult{
					RemoteID:   remoteID,
					Mutated:    false,
					Noop:       true,
					Diagnostic: "projection unchanged",
				}, nil
			}
			if err := a.closeIssue(remoteID, doneCloseReason(task.ID)); err != nil {
				return PushResult{}, err
			}
			a.projectionHashByID[task.ID] = currentHash
			a.sourceHashByID[task.ID] = payload.SourceHash
			a.provenanceByID[task.ID] = normalizedProvenance(task.Provenance)
			a.remoteIDByID[task.ID] = remoteID
			return PushResult{
				RemoteID:   remoteID,
				Mutated:    true,
				Diagnostic: "closed tracker issue for canonically completed task",
			}, nil
		}
		if isBeadsIssueClosed(currentIssue) {
			if err := a.reopenIssue(remoteID, reopenReason(task.ID)); err != nil {
				return PushResult{}, err
			}
			if _, err := a.updateIssueWithExternalRef(remoteID, task.ID, rendered.Title, description, acceptance, priority); err != nil {
				return PushResult{}, err
			}
			a.projectionHashByID[task.ID] = currentHash
			a.sourceHashByID[task.ID] = payload.SourceHash
			a.provenanceByID[task.ID] = normalizedProvenance(task.Provenance)
			a.remoteIDByID[task.ID] = remoteID
			return PushResult{
				RemoteID:   remoteID,
				Mutated:    true,
				Diagnostic: "reopened closed tracker issue with unchanged projection",
			}, nil
		}
		return PushResult{
			RemoteID:   remoteID,
			Mutated:    false,
			Noop:       true,
			Diagnostic: "projection unchanged",
		}, nil
	}
	if isLikelyBeadsIssueID(remoteID) {
		if isBeadsIssueClosed(currentIssue) && !shouldBeClosed(task) {
			if err := a.reopenIssue(remoteID, reopenReason(task.ID)); err != nil {
				return PushResult{}, err
			}
		}
		if _, err := a.updateIssueWithExternalRef(remoteID, task.ID, rendered.Title, description, acceptance, priority); err != nil {
			return PushResult{}, err
		}
	} else {
		issue, err := a.createIssueWithExternalRef(task.ID, rendered.Title, description, acceptance, priority)
		if err != nil {
			return PushResult{}, err
		}
		remoteID = issue.ID
	}
	if shouldBeClosed(task) {
		if err := a.closeIssue(remoteID, doneCloseReason(task.ID)); err != nil && !isBeadsNothingToDo(err) {
			return PushResult{}, err
		}
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

func (a *BeadsAdapter) SyncDependencies(_ context.Context, tasks map[string]TaskProjection) error {
	if len(tasks) == 0 {
		return nil
	}

	taskIDs := make([]string, 0, len(tasks))
	for id := range tasks {
		taskIDs = append(taskIDs, id)
	}
	sort.Strings(taskIDs)

	for _, taskID := range taskIDs {
		task := tasks[taskID]
		issueID := strings.TrimSpace(a.remoteIDByID[taskID])
		if !isLikelyBeadsIssueID(issueID) {
			continue
		}

		desired := make(map[string]struct{}, len(task.Dependencies))
		for _, depID := range task.Dependencies {
			depID = strings.TrimSpace(depID)
			if depID == "" || depID == taskID {
				continue
			}
			if _, ok := tasks[depID]; !ok {
				continue
			}
			depIssueID := strings.TrimSpace(a.remoteIDByID[depID])
			if !isLikelyBeadsIssueID(depIssueID) {
				existing, err := a.lookupIssueByExternalRef(depID)
				if err != nil {
					return fmt.Errorf("lookup dependency issue %q: %w", depID, err)
				}
				if isLikelyBeadsIssueID(existing.ID) {
					depIssueID = existing.ID
					a.remoteIDByID[depID] = depIssueID
				}
			}
			if !isLikelyBeadsIssueID(depIssueID) {
				continue
			}
			desired[depIssueID] = struct{}{}
		}

		currentDeps, err := a.listDependencies(issueID)
		if err != nil {
			return fmt.Errorf("list beads dependencies for %q: %w", taskID, err)
		}
		current := make(map[string]struct{}, len(currentDeps))
		for _, dep := range currentDeps {
			if strings.TrimSpace(dep.Type) != "" && dep.Type != "blocks" {
				continue
			}
			targetID := strings.TrimSpace(dep.DependsOnID)
			if targetID == "" || targetID == issueID {
				continue
			}
			current[targetID] = struct{}{}
		}

		toAdd := make([]string, 0)
		for depIssueID := range desired {
			if _, ok := current[depIssueID]; !ok {
				toAdd = append(toAdd, depIssueID)
			}
		}
		sort.Strings(toAdd)
		for _, depIssueID := range toAdd {
			if err := a.addDependency(issueID, depIssueID); err != nil {
				return fmt.Errorf("add beads dependency %q -> %q: %w", issueID, depIssueID, err)
			}
		}

		toRemove := make([]string, 0)
		for depIssueID := range current {
			if _, ok := desired[depIssueID]; !ok {
				toRemove = append(toRemove, depIssueID)
			}
		}
		sort.Strings(toRemove)
		for _, depIssueID := range toRemove {
			if err := a.removeDependency(issueID, depIssueID); err != nil {
				return fmt.Errorf("remove beads dependency %q -> %q: %w", issueID, depIssueID, err)
			}
		}
	}

	return nil
}

func (a *BeadsAdapter) MarkTaskStale(_ context.Context, id string, reason string) (PushResult, error) {
	remoteID := strings.TrimSpace(a.remoteIDByID[id])
	if !isLikelyBeadsIssueID(remoteID) {
		existing, err := a.lookupIssueByExternalRef(id)
		if err != nil {
			return PushResult{}, err
		}
		if isLikelyBeadsIssueID(existing.ID) {
			remoteID = existing.ID
		}
	}
	if !isLikelyBeadsIssueID(remoteID) {
		a.deleteTaskState(id)
		return PushResult{
			Noop:       true,
			Mutated:    false,
			Diagnostic: "tracker issue already absent for stale task",
		}, nil
	}
	currentDeps, err := a.listDependencies(remoteID)
	if err != nil {
		return PushResult{}, err
	}
	for _, dep := range currentDeps {
		depID := strings.TrimSpace(dep.DependsOnID)
		if depID == "" || depID == remoteID {
			continue
		}
		if err := a.removeDependency(remoteID, depID); err != nil {
			return PushResult{}, err
		}
	}
	if err := a.closeIssue(remoteID, staleCloseReason(reason)); err != nil {
		if isBeadsIssueNotFound(err) || isBeadsNothingToDo(err) {
			a.deleteTaskState(id)
			return PushResult{
				RemoteID:   remoteID,
				Noop:       true,
				Mutated:    false,
				Diagnostic: "tracker issue already closed or absent for stale task",
			}, nil
		}
		return PushResult{}, err
	}
	a.deleteTaskState(id)
	return PushResult{
		RemoteID:   remoteID,
		Mutated:    true,
		Diagnostic: "tracker issue closed for stale task",
	}, nil
}

func (a *BeadsAdapter) createIssue(title string, description string, acceptance string, priority int) (beadsIssue, error) {
	return a.createIssueWithExternalRef("", title, description, acceptance, priority)
}

func (a *BeadsAdapter) createIssueWithExternalRef(externalRef string, title string, description string, acceptance string, priority int) (beadsIssue, error) {
	args := []string{"create", "--title", title, "--type", "task", "--json"}
	if strings.TrimSpace(externalRef) != "" {
		args = append(args, "--external-ref", externalRef)
	}
	if priority > 0 {
		args = append(args, "--priority", fmt.Sprintf("%d", priority))
	}
	if strings.TrimSpace(description) != "" {
		args = append(args, "--description", description)
	}
	output, err := a.runBr(args...)
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

func (a *BeadsAdapter) updateIssue(id string, title string, description string, acceptance string, priority int) (beadsIssue, error) {
	return a.updateIssueWithExternalRef(id, "", title, description, acceptance, priority)
}

func (a *BeadsAdapter) updateIssueWithExternalRef(id string, externalRef string, title string, description string, acceptance string, priority int) (beadsIssue, error) {
	args := []string{"update", id, "--title", title, "--description", description, "--acceptance-criteria", acceptance}
	if strings.TrimSpace(externalRef) != "" {
		args = append(args, "--external-ref", externalRef)
	}
	if priority > 0 {
		args = append(args, "--priority", fmt.Sprintf("%d", priority))
	}
	args = append(args, "--json")
	output, err := a.runBr(args...)
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
	output, err := a.runBr("show", id, "--json")
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
	issues, err := a.listIssues("list", "--all", "--json")
	if err != nil {
		return beadsIssue{}, err
	}
	for _, issue := range issues {
		if strings.TrimSpace(issue.ExternalRef) == strings.TrimSpace(externalRef) {
			return issue, nil
		}
	}
	return beadsIssue{}, nil
}

func (a *BeadsAdapter) listDependencies(id string) ([]beadsDependency, error) {
	output, err := a.runBr("dep", "list", id, "--json")
	if err != nil {
		return nil, fmt.Errorf("br dep list %s: %w", id, err)
	}
	var deps []beadsDependency
	if err := json.Unmarshal(output, &deps); err != nil {
		return nil, fmt.Errorf("decode br dep list output: %w", err)
	}
	return deps, nil
}

func (a *BeadsAdapter) addDependency(issueID string, dependsOnID string) error {
	if _, err := a.runBr("dep", "add", issueID, dependsOnID, "--json"); err != nil {
		return fmt.Errorf("br dep add %s %s: %w", issueID, dependsOnID, err)
	}
	return nil
}

func (a *BeadsAdapter) removeDependency(issueID string, dependsOnID string) error {
	if _, err := a.runBr("dep", "remove", issueID, dependsOnID, "--json"); err != nil {
		return fmt.Errorf("br dep remove %s %s: %w", issueID, dependsOnID, err)
	}
	return nil
}

func (a *BeadsAdapter) ListStaleCandidates(_ context.Context, desiredIDs map[string]struct{}) ([]SyncManifestEntry, error) {
	issues, err := a.listIssues("list", "--all", "--json")
	if err != nil {
		return nil, err
	}
	candidates := make([]SyncManifestEntry, 0, len(issues))
	for _, issue := range issues {
		externalRef := strings.TrimSpace(issue.ExternalRef)
		if externalRef == "" {
			continue
		}
		if _, desired := desiredIDs[externalRef]; desired {
			continue
		}
		candidates = append(candidates, SyncManifestEntry{
			ID:       externalRef,
			RemoteID: strings.TrimSpace(issue.ID),
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].ID != candidates[j].ID {
			return candidates[i].ID < candidates[j].ID
		}
		return candidates[i].RemoteID < candidates[j].RemoteID
	})
	return candidates, nil
}

func (a *BeadsAdapter) ListCleanupCandidates(_ context.Context, planPath string, desiredIDs map[string]struct{}) ([]CleanupCandidate, error) {
	issues, err := a.listIssues("list", "--status", "open", "--json")
	if err != nil {
		return nil, err
	}
	normalizedPlanPath := normalizeCleanupPath(planPath)
	candidates := make([]CleanupCandidate, 0, len(issues))
	for _, issue := range issues {
		externalRef := strings.TrimSpace(issue.ExternalRef)
		if externalRef != "" {
			if _, desired := desiredIDs[externalRef]; desired {
				continue
			}
			candidates = append(candidates, CleanupCandidate{
				RemoteID:    strings.TrimSpace(issue.ID),
				Title:       strings.TrimSpace(issue.Title),
				ExternalRef: externalRef,
				Reason:      "external_ref is missing from current plan",
			})
			continue
		}
		sourcePath := parsePlanSourcePath(issue.Description)
		if sourcePath == "" {
			continue
		}
		normalizedSourcePath := normalizeCleanupPath(sourcePath)
		if normalizedSourcePath == "" || normalizedSourcePath == normalizedPlanPath {
			continue
		}
		candidates = append(candidates, CleanupCandidate{
			RemoteID:   strings.TrimSpace(issue.ID),
			Title:      strings.TrimSpace(issue.Title),
			SourcePath: sourcePath,
			Reason:     "plan-derived issue points at a different plan file",
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Reason != candidates[j].Reason {
			return candidates[i].Reason < candidates[j].Reason
		}
		if candidates[i].ExternalRef != candidates[j].ExternalRef {
			return candidates[i].ExternalRef < candidates[j].ExternalRef
		}
		if candidates[i].SourcePath != candidates[j].SourcePath {
			return candidates[i].SourcePath < candidates[j].SourcePath
		}
		return candidates[i].RemoteID < candidates[j].RemoteID
	})
	return candidates, nil
}

func (a *BeadsAdapter) CleanupCandidate(_ context.Context, candidate CleanupCandidate) error {
	if strings.TrimSpace(candidate.RemoteID) == "" {
		return fmt.Errorf("cleanup candidate requires remote id")
	}
	if err := a.closeIssue(candidate.RemoteID, cleanupCloseReason(candidate)); err != nil {
		if isBeadsIssueNotFound(err) || isBeadsNothingToDo(err) {
			return nil
		}
		return err
	}
	return nil
}

func (a *BeadsAdapter) listIssues(args ...string) ([]beadsIssue, error) {
	output, err := a.runBr(args...)
	if err != nil {
		return nil, fmt.Errorf("br list: %w", err)
	}
	var issues []beadsIssue
	if err := json.Unmarshal(output, &issues); err != nil {
		return nil, fmt.Errorf("decode br list output: %w", err)
	}
	return issues, nil
}

func (a *BeadsAdapter) closeIssue(id string, reason string) error {
	args := []string{"close", id, "--reason", reason, "--json"}
	if _, err := a.runBr(args...); err != nil {
		return fmt.Errorf("br close %s: %w", id, err)
	}
	return nil
}

func (a *BeadsAdapter) reopenIssue(id string, reason string) error {
	args := []string{"reopen", id, "--reason", reason, "--json"}
	if _, err := a.runBr(args...); err != nil {
		return fmt.Errorf("br reopen %s: %w", id, err)
	}
	return nil
}

func (a *BeadsAdapter) runBr(args ...string) ([]byte, error) {
	if strings.TrimSpace(a.dbPath) == "" {
		return runBrCommand(args...)
	}
	dbDir := filepath.Dir(a.dbPath)
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return nil, fmt.Errorf("prepare beads db dir: %w", err)
	}
	if err := ensureBeadsWorkspaceMetadata(dbDir, filepath.Base(a.dbPath)); err != nil {
		return nil, err
	}
	withDB := make([]string, 0, len(args)+2)
	withDB = append(withDB, args[0], "--db", a.dbPath)
	withDB = append(withDB, args[1:]...)
	return runBrCommand(withDB...)
}

func ensureBeadsWorkspaceMetadata(beadsDir string, dbFile string) error {
	metadataPath := filepath.Join(beadsDir, "metadata.json")
	if _, err := os.Stat(metadataPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat beads metadata: %w", err)
	}

	payload := struct {
		Database    string `json:"database"`
		JSONLExport string `json:"jsonl_export"`
	}{
		Database:    strings.TrimSpace(dbFile),
		JSONLExport: "issues.jsonl",
	}
	if payload.Database == "" {
		payload.Database = "beads.db"
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal beads metadata: %w", err)
	}
	raw = append(raw, '\n')
	if err := fsio.WriteFileAtomic(metadataPath, raw, 0o644); err != nil {
		return fmt.Errorf("write beads metadata: %w", err)
	}
	return nil
}

func (a *BeadsAdapter) deleteTaskState(id string) {
	delete(a.projectionHashByID, id)
	delete(a.sourceHashByID, id)
	delete(a.provenanceByID, id)
	delete(a.remoteIDByID, id)
	delete(a.runtimeByID, id)
	delete(a.lastSeenRuntime, id)
	delete(a.pushFailuresByID, id)
}

func isLikelyBeadsIssueID(id string) bool {
	trimmed := strings.TrimSpace(id)
	return beadsIssueIDPattern.MatchString(trimmed)
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

func isBeadsNothingToDo(err error) bool {
	if err == nil {
		return false
	}
	if strings.Contains(err.Error(), `"code": "NOTHING_TO_DO"`) || strings.Contains(err.Error(), `"code":"NOTHING_TO_DO"`) {
		return true
	}
	raw := err.Error()
	if idx := strings.Index(raw, "{"); idx >= 0 {
		raw = raw[idx:]
	}
	var payload beadsErrorEnvelope
	if jsonErr := json.Unmarshal([]byte(raw), &payload); jsonErr == nil {
		return strings.EqualFold(strings.TrimSpace(payload.Error.Code), "NOTHING_TO_DO")
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "nothing_to_do") || strings.Contains(lower, "nothing to do")
}

func staleCloseReason(reason string) string {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return "PlanMark sync marked this task stale"
	}
	return "PlanMark sync marked this task stale: " + trimmed
}

func reopenReason(taskID string) string {
	trimmed := strings.TrimSpace(taskID)
	if trimmed == "" {
		return "PlanMark sync restored current plan task"
	}
	return "PlanMark sync restored current plan task: " + trimmed
}

func doneCloseReason(taskID string) string {
	trimmed := strings.TrimSpace(taskID)
	if trimmed == "" {
		return "PlanMark sync marked this task done in the canonical plan"
	}
	return "PlanMark sync marked this task done in the canonical plan: " + trimmed
}

func isBeadsIssueClosed(issue beadsIssue) bool {
	return strings.EqualFold(strings.TrimSpace(issue.Status), "closed")
}

func cleanupCloseReason(candidate CleanupCandidate) string {
	reason := strings.TrimSpace(candidate.Reason)
	if reason == "" {
		reason = "cleanup candidate"
	}
	return "PlanMark cleanup closed non-plan bead: " + reason
}

func parsePlanSourcePath(description string) string {
	for _, line := range strings.Split(description, "\n") {
		matches := sourceLinePattern.FindStringSubmatch(strings.TrimSpace(line))
		if len(matches) == 4 {
			return strings.TrimSpace(matches[1])
		}
	}
	return ""
}

func normalizeCleanupPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	if abs, err := filepath.Abs(trimmed); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(trimmed)
}

func beadsPriorityForHorizon(horizon string) int {
	switch strings.ToLower(strings.TrimSpace(horizon)) {
	case "now":
		return 1
	case "next":
		return 2
	case "later":
		return 3
	default:
		return 4
	}
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
		projectionVersion = ProjectionSchemaVersionV03
	}

	return BeadsProjectionPayload{
		ProjectionSchemaVersion: projectionVersion,
		ID:                      task.ID,
		Title:                   rendered.Title,
		CanonicalStatus:         normalizeCanonicalTaskStatus(task.CanonicalStatus),
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

func shouldBeClosed(task TaskProjection) bool {
	return normalizeCanonicalTaskStatus(task.CanonicalStatus) == "done"
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
