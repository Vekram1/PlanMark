package syncplanner

import (
	"reflect"
	"testing"
)

func TestBeadsDeletionPolicyDefaultMarkStale(t *testing.T) {
	policy, err := ParseDeletionPolicy("")
	if err != nil {
		t.Fatalf("parse default deletion policy: %v", err)
	}
	if policy != DeletionPolicyMarkStale {
		t.Fatalf("expected default policy %q, got %q", DeletionPolicyMarkStale, policy)
	}
}

func TestParseDeletionPolicySupportsAllKnownValues(t *testing.T) {
	tests := []DeletionPolicy{
		DeletionPolicyMarkStale,
		DeletionPolicyClose,
		DeletionPolicyDetach,
		DeletionPolicyDelete,
	}
	for _, tt := range tests {
		policy, err := ParseDeletionPolicy(string(tt))
		if err != nil {
			t.Fatalf("parse %q: %v", tt, err)
		}
		if policy != tt {
			t.Fatalf("expected %q, got %q", tt, policy)
		}
	}
}

func TestParseDeletionPolicyRejectsUnknownValue(t *testing.T) {
	if _, err := ParseDeletionPolicy("archive"); err == nil {
		t.Fatalf("expected error for invalid policy")
	}
}

func TestBeadsSyncPlannerStableOps(t *testing.T) {
	desired := []DesiredProjection{
		{ID: "task.create", ProjectionHash: "h.create"},
		{ID: "task.update", ProjectionHash: "h.new"},
		{ID: "task.noop", ProjectionHash: "h.same"},
		{ID: "task.conflict", ProjectionHash: ""},
		{ID: "task.dupe", ProjectionHash: "h.dupe.1"},
		{ID: "task.dupe", ProjectionHash: "h.dupe.2"},
	}
	prior := []PriorProjection{
		{ID: "task.update", ProjectionHash: "h.old"},
		{ID: "task.noop", ProjectionHash: "h.same"},
		{ID: "task.stale", ProjectionHash: "h.stale"},
		{ID: "task.dupe.prior", ProjectionHash: "h.prior.1"},
		{ID: "task.dupe.prior", ProjectionHash: "h.prior.2"},
	}

	gotA := PlanSyncOps(desired, prior, DeletionPolicyMarkStale)
	gotB := PlanSyncOps(
		[]DesiredProjection{
			{ID: "task.dupe", ProjectionHash: "h.dupe.2"},
			{ID: "task.create", ProjectionHash: "h.create"},
			{ID: "task.noop", ProjectionHash: "h.same"},
			{ID: "task.update", ProjectionHash: "h.new"},
			{ID: "task.conflict", ProjectionHash: ""},
			{ID: "task.dupe", ProjectionHash: "h.dupe.1"},
		},
		[]PriorProjection{
			{ID: "task.stale", ProjectionHash: "h.stale"},
			{ID: "task.dupe.prior", ProjectionHash: "h.prior.2"},
			{ID: "task.update", ProjectionHash: "h.old"},
			{ID: "task.noop", ProjectionHash: "h.same"},
			{ID: "task.dupe.prior", ProjectionHash: "h.prior.1"},
		},
		DeletionPolicyMarkStale,
	)

	if !reflect.DeepEqual(gotA, gotB) {
		t.Fatalf("expected stable deterministic operations across input order\nA=%#v\nB=%#v", gotA, gotB)
	}

	seenKinds := map[OperationKind]bool{}
	for _, op := range gotA {
		seenKinds[op.Kind] = true
	}
	for _, kind := range []OperationKind{
		OperationCreate,
		OperationUpdate,
		OperationNoop,
		OperationMarkStale,
		OperationConflict,
	} {
		if !seenKinds[kind] {
			t.Fatalf("expected operation kind %q in plan, got %#v", kind, gotA)
		}
	}
}
