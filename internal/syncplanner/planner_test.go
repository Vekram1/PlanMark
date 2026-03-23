package syncplanner

import (
	"fmt"
	"reflect"
	"sort"
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

func TestPlanSyncOpsSingleIDStateMatrixExhaustive(t *testing.T) {
	baseProv := comparableProvenance("task.one", "hash.same")
	changedProv := comparableProvenance("task.one.changed", "hash.changed")

	desiredModes := []struct {
		name string
		rows []DesiredProjection
	}{
		{name: "absent"},
		{name: "create", rows: []DesiredProjection{{ID: "task.one", ProjectionHash: "hash.same", Provenance: baseProv}}},
		{name: "missing-hash", rows: []DesiredProjection{{ID: "task.one", ProjectionHash: "", Provenance: baseProv}}},
		{name: "duplicate", rows: []DesiredProjection{
			{ID: "task.one", ProjectionHash: "hash.same", Provenance: baseProv},
			{ID: "task.one", ProjectionHash: "hash.alt", Provenance: baseProv},
		}},
	}

	priorModes := []struct {
		name string
		rows []PriorProjection
	}{
		{name: "absent"},
		{name: "same", rows: []PriorProjection{{ID: "task.one", ProjectionHash: "hash.same", Provenance: baseProv}}},
		{name: "different", rows: []PriorProjection{{ID: "task.one", ProjectionHash: "hash.old", Provenance: baseProv}}},
		{name: "missing-hash", rows: []PriorProjection{{ID: "task.one", ProjectionHash: "", Provenance: baseProv}}},
		{name: "duplicate", rows: []PriorProjection{
			{ID: "task.one", ProjectionHash: "hash.old", Provenance: baseProv},
			{ID: "task.one", ProjectionHash: "hash.older", Provenance: baseProv},
		}},
		{name: "provenance-mismatch", rows: []PriorProjection{{ID: "task.one", ProjectionHash: "hash.old", Provenance: changedProv}}},
	}

	for _, desiredMode := range desiredModes {
		for _, priorMode := range priorModes {
			name := fmt.Sprintf("desired=%s/prior=%s", desiredMode.name, priorMode.name)
			t.Run(name, func(t *testing.T) {
				ops := PlanSyncOps(desiredMode.rows, priorMode.rows, DeletionPolicyMarkStale)
				wantKind := expectedSingleIDOperationKind(desiredMode.name, priorMode.name)
				if wantKind == "" {
					if len(ops) != 0 {
						t.Fatalf("expected no operations, got %#v", ops)
					}
					return
				}
				if len(ops) != 1 {
					t.Fatalf("expected exactly one operation, got %#v", ops)
				}
				if ops[0].Kind != wantKind {
					t.Fatalf("unexpected operation kind: got %q want %q (%#v)", ops[0].Kind, wantKind, ops)
				}
				if ops[0].ID != "task.one" {
					t.Fatalf("expected task.one op, got %#v", ops[0])
				}
			})
		}
	}
}

func TestPlanSyncOpsOrderIndependenceExhaustiveTwoIDs(t *testing.T) {
	type scenario struct {
		name    string
		desired []DesiredProjection
		prior   []PriorProjection
	}

	scenarios := []scenario{
		{
			name: "create",
			desired: []DesiredProjection{
				{ID: "task.id", ProjectionHash: "hash.create", Provenance: comparableProvenance("task.id", "hash.create")},
			},
		},
		{
			name: "update",
			desired: []DesiredProjection{
				{ID: "task.id", ProjectionHash: "hash.new", Provenance: comparableProvenance("task.id", "hash.new")},
			},
			prior: []PriorProjection{
				{ID: "task.id", ProjectionHash: "hash.old", Provenance: comparableProvenance("task.id", "hash.old")},
			},
		},
		{
			name: "noop",
			desired: []DesiredProjection{
				{ID: "task.id", ProjectionHash: "hash.same", Provenance: comparableProvenance("task.id", "hash.same")},
			},
			prior: []PriorProjection{
				{ID: "task.id", ProjectionHash: "hash.same", Provenance: comparableProvenance("task.id", "hash.same")},
			},
		},
		{
			name: "stale",
			desired: []DesiredProjection{
				{ID: "task.id", ProjectionHash: "hash.new", Provenance: comparableProvenance("task.id.new", "hash.new")},
			},
			prior: []PriorProjection{
				{ID: "task.id", ProjectionHash: "hash.old", Provenance: comparableProvenance("task.id.old", "hash.old")},
			},
		},
		{
			name: "conflict",
			desired: []DesiredProjection{
				{ID: "task.id", ProjectionHash: "", Provenance: comparableProvenance("task.id", "hash.any")},
			},
		},
	}

	for _, left := range scenarios {
		for _, right := range scenarios {
			name := left.name + "__" + right.name
			t.Run(name, func(t *testing.T) {
				desired := append(desiredWithID("task.a", left.desired), desiredWithID("task.b", right.desired)...)
				prior := append(priorWithID("task.a", left.prior), priorWithID("task.b", right.prior)...)

				var baseline []Operation
				for _, desiredPerm := range permuteDesired(desired) {
					for _, priorPerm := range permutePrior(prior) {
						got := PlanSyncOps(desiredPerm, priorPerm, DeletionPolicyMarkStale)
						if baseline == nil {
							baseline = got
							continue
						}
						if !reflect.DeepEqual(baseline, got) {
							t.Fatalf("non-deterministic ops for permutations\nbaseline=%#v\ngot=%#v", baseline, got)
						}
					}
				}
			})
		}
	}
}

func TestPlanSyncOpsProducesAtMostOneOperationPerIDExhaustive(t *testing.T) {
	desiredSets := [][]DesiredProjection{
		nil,
		{{ID: "task.one", ProjectionHash: "hash.one", Provenance: comparableProvenance("task.one", "hash.one")}},
		{{ID: "task.one", ProjectionHash: "", Provenance: comparableProvenance("task.one", "hash.one")}},
		{
			{ID: "task.one", ProjectionHash: "hash.one", Provenance: comparableProvenance("task.one", "hash.one")},
			{ID: "task.one", ProjectionHash: "hash.two", Provenance: comparableProvenance("task.one", "hash.one")},
		},
	}
	priorSets := [][]PriorProjection{
		nil,
		{{ID: "task.one", ProjectionHash: "hash.one", Provenance: comparableProvenance("task.one", "hash.one")}},
		{{ID: "task.one", ProjectionHash: "hash.zero", Provenance: comparableProvenance("task.one.old", "hash.zero")}},
		{
			{ID: "task.one", ProjectionHash: "hash.zero", Provenance: comparableProvenance("task.one", "hash.zero")},
			{ID: "task.one", ProjectionHash: "hash.older", Provenance: comparableProvenance("task.one", "hash.zero")},
		},
	}

	for i, desired := range desiredSets {
		for j, prior := range priorSets {
			name := fmt.Sprintf("desired=%d/prior=%d", i, j)
			t.Run(name, func(t *testing.T) {
				ops := PlanSyncOps(desired, prior, DeletionPolicyMarkStale)
				seen := map[string]int{}
				for _, op := range ops {
					seen[op.ID]++
					if seen[op.ID] > 1 {
						t.Fatalf("expected at most one operation per id, got %#v", ops)
					}
				}
			})
		}
	}
}

func expectedSingleIDOperationKind(desiredMode, priorMode string) OperationKind {
	switch desiredMode {
	case "absent":
		switch priorMode {
		case "absent":
			return ""
		case "duplicate":
			return OperationConflict
		}
		return OperationMarkStale
	case "duplicate":
		return OperationConflict
	case "missing-hash":
		return OperationConflict
	case "create":
		switch priorMode {
		case "absent":
			return OperationCreate
		case "duplicate", "missing-hash":
			return OperationConflict
		case "provenance-mismatch":
			return OperationMarkStale
		case "same":
			return OperationNoop
		case "different":
			return OperationUpdate
		}
	}
	return ""
}

func comparableProvenance(seed string, sourceHash string) tracker.TaskProvenance {
	line := len(seed) + 1
	return tracker.TaskProvenance{
		NodeRef:    "plan.md|checkbox|" + seed + "#1",
		Path:       "plan.md",
		StartLine:  line,
		EndLine:    line,
		SourceHash: sourceHash,
		CompileID:  "compile-" + seed,
	}
}

func desiredWithID(id string, items []DesiredProjection) []DesiredProjection {
	out := make([]DesiredProjection, 0, len(items))
	for _, item := range items {
		cloned := item
		cloned.ID = id
		if cloned.Provenance.NodeRef != "" {
			cloned.Provenance = comparableProvenance(id, cloned.Provenance.SourceHash)
		}
		out = append(out, cloned)
	}
	return out
}

func priorWithID(id string, items []PriorProjection) []PriorProjection {
	out := make([]PriorProjection, 0, len(items))
	for _, item := range items {
		cloned := item
		cloned.ID = id
		if cloned.Provenance.NodeRef != "" {
			cloned.Provenance = comparableProvenance(id, cloned.Provenance.SourceHash)
		}
		out = append(out, cloned)
	}
	return out
}

func permuteDesired(items []DesiredProjection) [][]DesiredProjection {
	if len(items) == 0 {
		return [][]DesiredProjection{{}}
	}
	return uniqueDesiredPermutations(items)
}

func permutePrior(items []PriorProjection) [][]PriorProjection {
	if len(items) == 0 {
		return [][]PriorProjection{{}}
	}
	return uniquePriorPermutations(items)
}

func uniqueDesiredPermutations(items []DesiredProjection) [][]DesiredProjection {
	out := make([][]DesiredProjection, 0)
	seen := map[string]struct{}{}
	var walk func(int, []DesiredProjection)
	walk = func(idx int, cur []DesiredProjection) {
		if idx == len(cur) {
			key := fmt.Sprintf("%#v", cur)
			if _, ok := seen[key]; ok {
				return
			}
			seen[key] = struct{}{}
			out = append(out, append([]DesiredProjection(nil), cur...))
			return
		}
		for i := idx; i < len(cur); i++ {
			cur[idx], cur[i] = cur[i], cur[idx]
			walk(idx+1, cur)
			cur[idx], cur[i] = cur[i], cur[idx]
		}
	}
	walk(0, append([]DesiredProjection(nil), items...))
	sort.Slice(out, func(i, j int) bool {
		return fmt.Sprintf("%#v", out[i]) < fmt.Sprintf("%#v", out[j])
	})
	return out
}

func uniquePriorPermutations(items []PriorProjection) [][]PriorProjection {
	out := make([][]PriorProjection, 0)
	seen := map[string]struct{}{}
	var walk func(int, []PriorProjection)
	walk = func(idx int, cur []PriorProjection) {
		if idx == len(cur) {
			key := fmt.Sprintf("%#v", cur)
			if _, ok := seen[key]; ok {
				return
			}
			seen[key] = struct{}{}
			out = append(out, append([]PriorProjection(nil), cur...))
			return
		}
		for i := idx; i < len(cur); i++ {
			cur[idx], cur[i] = cur[i], cur[idx]
			walk(idx+1, cur)
			cur[idx], cur[i] = cur[i], cur[idx]
		}
	}
	walk(0, append([]PriorProjection(nil), items...))
	sort.Slice(out, func(i, j int) bool {
		return fmt.Sprintf("%#v", out[i]) < fmt.Sprintf("%#v", out[j])
	})
	return out
}
