package syncplanner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/tracker"
)

type DeletionPolicy string

const (
	DeletionPolicyMarkStale DeletionPolicy = "mark-stale"
	DeletionPolicyClose     DeletionPolicy = "close"
	DeletionPolicyDetach    DeletionPolicy = "detach"
	DeletionPolicyDelete    DeletionPolicy = "delete"
)

func DefaultDeletionPolicy() DeletionPolicy {
	return DeletionPolicyMarkStale
}

func ParseDeletionPolicy(raw string) (DeletionPolicy, error) {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	if trimmed == "" {
		return DefaultDeletionPolicy(), nil
	}

	switch DeletionPolicy(trimmed) {
	case DeletionPolicyMarkStale, DeletionPolicyClose, DeletionPolicyDetach, DeletionPolicyDelete:
		return DeletionPolicy(trimmed), nil
	default:
		return "", fmt.Errorf("invalid deletion policy %q (supported: %s)", raw, strings.Join(SupportedDeletionPolicies(), ","))
	}
}

func SupportedDeletionPolicies() []string {
	policies := []string{
		string(DeletionPolicyMarkStale),
		string(DeletionPolicyClose),
		string(DeletionPolicyDetach),
		string(DeletionPolicyDelete),
	}
	sort.Strings(policies)
	return policies
}

type OperationKind string

const (
	OperationCreate    OperationKind = "create"
	OperationUpdate    OperationKind = "update"
	OperationNoop      OperationKind = "no-op"
	OperationMarkStale OperationKind = "mark-stale"
	OperationConflict  OperationKind = "conflict"
)

type DesiredProjection struct {
	ID             string
	ProjectionHash string
	Provenance     tracker.TaskProvenance
}

type PriorProjection struct {
	ID             string
	ProjectionHash string
	Provenance     tracker.TaskProvenance
}

type Operation struct {
	Kind     OperationKind `json:"kind"`
	ID       string        `json:"id"`
	Reason   string        `json:"reason"`
	Priority int           `json:"priority"`
}

func PlanSyncOps(desired []DesiredProjection, prior []PriorProjection, deletionPolicy DeletionPolicy) []Operation {
	desiredByID := make(map[string]DesiredProjection, len(desired))
	ops := make([]Operation, 0, len(desired)+len(prior))
	desiredHashesByID := make(map[string][]string, len(desired))
	desiredProvenanceByID := make(map[string]tracker.TaskProvenance, len(desired))
	desiredConflictReasons := make(map[string]string, len(desired))
	for _, d := range desired {
		id := strings.TrimSpace(d.ID)
		if id == "" {
			continue
		}
		desiredHashesByID[id] = append(desiredHashesByID[id], strings.TrimSpace(d.ProjectionHash))
		if _, exists := desiredProvenanceByID[id]; !exists {
			desiredProvenanceByID[id] = d.Provenance
		}
	}
	desiredIDs := make([]string, 0, len(desiredHashesByID))
	for id := range desiredHashesByID {
		desiredIDs = append(desiredIDs, id)
	}
	sort.Strings(desiredIDs)
	for _, id := range desiredIDs {
		hashes := desiredHashesByID[id]
		if len(hashes) > 1 {
			desiredConflictReasons[id] = "duplicate desired id"
			continue
		}
		hash := hashes[0]
		if hash == "" {
			desiredConflictReasons[id] = "missing desired projection hash"
			continue
		}
		desiredByID[id] = DesiredProjection{ID: id, ProjectionHash: hash, Provenance: desiredProvenanceByID[id]}
	}
	priorByID := make(map[string]PriorProjection, len(prior))
	priorHashesByID := make(map[string][]string, len(prior))
	priorProvenanceByID := make(map[string]tracker.TaskProvenance, len(prior))
	priorConflictReasons := make(map[string]string, len(prior))
	for _, p := range prior {
		id := strings.TrimSpace(p.ID)
		if id == "" {
			continue
		}
		priorHashesByID[id] = append(priorHashesByID[id], strings.TrimSpace(p.ProjectionHash))
		if _, exists := priorProvenanceByID[id]; !exists {
			priorProvenanceByID[id] = p.Provenance
		}
	}
	priorIDs := make([]string, 0, len(priorHashesByID))
	for id := range priorHashesByID {
		priorIDs = append(priorIDs, id)
	}
	sort.Strings(priorIDs)
	for _, id := range priorIDs {
		hashes := priorHashesByID[id]
		if len(hashes) > 1 {
			priorConflictReasons[id] = "duplicate prior id"
			continue
		}
		priorByID[id] = PriorProjection{
			ID:             id,
			ProjectionHash: hashes[0],
			Provenance:     priorProvenanceByID[id],
		}
	}

	conflictIDs := make(map[string]struct{}, len(desiredConflictReasons)+len(priorConflictReasons))
	for id, reason := range desiredConflictReasons {
		conflictIDs[id] = struct{}{}
		ops = append(ops, Operation{
			Kind:     OperationConflict,
			ID:       id,
			Reason:   reason,
			Priority: 0,
		})
	}
	for id, reason := range priorConflictReasons {
		if _, exists := conflictIDs[id]; exists {
			continue
		}
		conflictIDs[id] = struct{}{}
		ops = append(ops, Operation{
			Kind:     OperationConflict,
			ID:       id,
			Reason:   reason,
			Priority: 0,
		})
	}

	for id, d := range desiredByID {
		if _, conflicted := conflictIDs[id]; conflicted {
			continue
		}
		p, ok := priorByID[id]
		if !ok {
			ops = append(ops, Operation{
				Kind:     OperationCreate,
				ID:       id,
				Reason:   "missing in prior projection set",
				Priority: 1,
			})
			continue
		}
		if p.ProjectionHash == "" {
			ops = append(ops, Operation{
				Kind:     OperationConflict,
				ID:       id,
				Reason:   "missing prior projection hash",
				Priority: 0,
			})
			continue
		}
		if provenanceComparable(p.Provenance) && provenanceComparable(d.Provenance) {
			if mismatch := provenanceMismatchReason(p.Provenance, d.Provenance); mismatch != "" {
				ops = append(ops, Operation{
					Kind:     OperationMarkStale,
					ID:       id,
					Reason:   "stale provenance mismatch: " + mismatch,
					Priority: 2,
				})
				continue
			}
		}
		if p.ProjectionHash == d.ProjectionHash {
			ops = append(ops, Operation{
				Kind:     OperationNoop,
				ID:       id,
				Reason:   "projection unchanged",
				Priority: 4,
			})
			continue
		}
		ops = append(ops, Operation{
			Kind:     OperationUpdate,
			ID:       id,
			Reason:   "projection hash changed",
			Priority: 2,
		})
	}

	staleKind := OperationMarkStale
	if deletionPolicy == DeletionPolicyDelete || deletionPolicy == DeletionPolicyClose || deletionPolicy == DeletionPolicyDetach {
		staleKind = OperationMarkStale
	}
	for id := range priorByID {
		if _, conflicted := conflictIDs[id]; conflicted {
			continue
		}
		if _, ok := desiredByID[id]; ok {
			continue
		}
		ops = append(ops, Operation{
			Kind:     staleKind,
			ID:       id,
			Reason:   "present in prior projection set but missing in desired",
			Priority: 3,
		})
	}

	sort.Slice(ops, func(i, j int) bool {
		if ops[i].Kind != ops[j].Kind {
			return ops[i].Kind < ops[j].Kind
		}
		if ops[i].Priority != ops[j].Priority {
			return ops[i].Priority < ops[j].Priority
		}
		if ops[i].ID != ops[j].ID {
			return ops[i].ID < ops[j].ID
		}
		return ops[i].Reason < ops[j].Reason
	})
	return ops
}

func ProjectionHashForTask(task tracker.TaskProjection) (string, error) {
	payload, err := tracker.BuildProjectionPayload(task)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal projection payload: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func provenanceComparable(p tracker.TaskProvenance) bool {
	return strings.TrimSpace(p.NodeRef) != "" &&
		strings.TrimSpace(p.Path) != "" &&
		p.StartLine > 0 &&
		p.EndLine >= p.StartLine &&
		strings.TrimSpace(p.SourceHash) != ""
}

func provenanceMismatchReason(prior tracker.TaskProvenance, desired tracker.TaskProvenance) string {
	diffs := make([]string, 0, 6)
	if strings.TrimSpace(prior.NodeRef) != strings.TrimSpace(desired.NodeRef) {
		diffs = append(diffs, "node_ref")
	}
	if strings.TrimSpace(prior.Path) != strings.TrimSpace(desired.Path) {
		diffs = append(diffs, "path")
	}
	if prior.StartLine != desired.StartLine || prior.EndLine != desired.EndLine {
		diffs = append(diffs, "range")
	}
	if strings.TrimSpace(prior.SourceHash) != strings.TrimSpace(desired.SourceHash) {
		diffs = append(diffs, "source_hash")
	}
	return strings.Join(diffs, ",")
}
