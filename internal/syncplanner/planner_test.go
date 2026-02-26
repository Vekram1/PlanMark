package syncplanner

import (
	"reflect"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/tracker"
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

func TestPlanSyncOpsDuplicateDesiredIsDeterministicAndConflicted(t *testing.T) {
	prior := []PriorProjection{{ID: "task.same", ProjectionHash: "h.prior"}}

	gotA := PlanSyncOps(
		[]DesiredProjection{
			{ID: "task.same", ProjectionHash: "h.new.1"},
			{ID: "task.same", ProjectionHash: "h.new.2"},
		},
		prior,
		DeletionPolicyMarkStale,
	)
	gotB := PlanSyncOps(
		[]DesiredProjection{
			{ID: "task.same", ProjectionHash: "h.new.2"},
			{ID: "task.same", ProjectionHash: "h.new.1"},
		},
		prior,
		DeletionPolicyMarkStale,
	)

	if !reflect.DeepEqual(gotA, gotB) {
		t.Fatalf("expected deterministic result for duplicate desired ids regardless of input order\nA=%#v\nB=%#v", gotA, gotB)
	}

	conflicts := 0
	for _, op := range gotA {
		if op.Kind == OperationConflict && op.ID == "task.same" {
			conflicts++
		}
		if op.Kind == OperationCreate || op.Kind == OperationUpdate || op.Kind == OperationNoop {
			t.Fatalf("did not expect non-conflict op for duplicate desired id; got %#v", gotA)
		}
	}
	if conflicts != 1 {
		t.Fatalf("expected exactly one conflict for duplicate desired id, got %d (%#v)", conflicts, gotA)
	}
}

func TestBeadProvenanceMismatchMarkedStale(t *testing.T) {
	desired := []DesiredProjection{
		{
			ID:             "task.same",
			ProjectionHash: "h.new",
			Provenance: tracker.TaskProvenance{
				NodeRef:    "plan.md|checkbox|new#1",
				Path:       "plan.md",
				StartLine:  11,
				EndLine:    11,
				SourceHash: "newhash",
				CompileID:  "compile-new",
			},
		},
	}
	prior := []PriorProjection{
		{
			ID:             "task.same",
			ProjectionHash: "h.old",
			Provenance: tracker.TaskProvenance{
				NodeRef:    "plan.md|checkbox|old#1",
				Path:       "plan.md",
				StartLine:  7,
				EndLine:    7,
				SourceHash: "oldhash",
				CompileID:  "compile-old",
			},
		},
	}

	ops := PlanSyncOps(desired, prior, DeletionPolicyMarkStale)
	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %#v", ops)
	}
	if ops[0].Kind != OperationMarkStale {
		t.Fatalf("expected mark-stale op, got %#v", ops[0])
	}
	if ops[0].ID != "task.same" {
		t.Fatalf("expected task.same op, got %#v", ops[0])
	}
	if ops[0].Reason == "" || ops[0].Reason == "present in prior projection set but missing in desired" {
		t.Fatalf("expected explicit provenance mismatch reason, got %#v", ops[0])
	}
}

func TestBeadCompileIDDifferenceDoesNotMarkStale(t *testing.T) {
	desired := []DesiredProjection{
		{
			ID:             "task.same",
			ProjectionHash: "h.same",
			Provenance: tracker.TaskProvenance{
				NodeRef:    "plan.md|checkbox|same#1",
				Path:       "plan.md",
				StartLine:  11,
				EndLine:    11,
				SourceHash: "samehash",
				CompileID:  "compile-new",
			},
		},
	}
	prior := []PriorProjection{
		{
			ID:             "task.same",
			ProjectionHash: "h.same",
			Provenance: tracker.TaskProvenance{
				NodeRef:    "plan.md|checkbox|same#1",
				Path:       "plan.md",
				StartLine:  11,
				EndLine:    11,
				SourceHash: "samehash",
				CompileID:  "compile-old",
			},
		},
	}

	ops := PlanSyncOps(desired, prior, DeletionPolicyMarkStale)
	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %#v", ops)
	}
	if ops[0].Kind != OperationNoop {
		t.Fatalf("expected noop when only compile id differs, got %#v", ops[0])
	}
}
