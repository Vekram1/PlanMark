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
}

type PriorProjection struct {
	ID             string
	ProjectionHash string
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

	for _, d := range desired {
		id := strings.TrimSpace(d.ID)
		if id == "" {
			continue
		}
		hash := strings.TrimSpace(d.ProjectionHash)
		if hash == "" {
			ops = append(ops, Operation{
				Kind:     OperationConflict,
				ID:       id,
				Reason:   "missing desired projection hash",
				Priority: 0,
			})
			continue
		}
		if _, exists := desiredByID[id]; exists {
			ops = append(ops, Operation{
				Kind:     OperationConflict,
				ID:       id,
				Reason:   "duplicate desired id",
				Priority: 0,
			})
			continue
		}
		desiredByID[id] = DesiredProjection{ID: id, ProjectionHash: hash}
	}

	priorByID := make(map[string]PriorProjection, len(prior))
	for _, p := range prior {
		id := strings.TrimSpace(p.ID)
		if id == "" {
			continue
		}
		if _, exists := priorByID[id]; exists {
			ops = append(ops, Operation{
				Kind:     OperationConflict,
				ID:       id,
				Reason:   "duplicate prior id",
				Priority: 0,
			})
			continue
		}
		priorByID[id] = PriorProjection{
			ID:             id,
			ProjectionHash: strings.TrimSpace(p.ProjectionHash),
		}
	}

	for id, d := range desiredByID {
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
