package syncplanner

import (
	"reflect"
	"strings"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/tracker"
)

func FuzzParseDeletionPolicyDeterminism(f *testing.F) {
	for _, seed := range []string{"", "mark-stale", "close", "detach", "delete", "archive"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		first, errA := ParseDeletionPolicy(raw)
		second, errB := ParseDeletionPolicy(raw)
		if (errA != nil) != (errB != nil) {
			t.Fatalf("nondeterministic deletion policy error state: %v vs %v", errA, errB)
		}
		if errA != nil {
			if errA.Error() != errB.Error() {
				t.Fatalf("nondeterministic deletion policy error text: %q vs %q", errA.Error(), errB.Error())
			}
			return
		}
		if first != second {
			t.Fatalf("nondeterministic deletion policy result: %q vs %q", first, second)
		}
	})
}

func FuzzPlanSyncOpsDeterminism(f *testing.F) {
	f.Add([]byte("task.a|hash.a\n task.b | hash.b \n"), []byte("task.b|hash.old\n task.c|hash.c \n"), "mark-stale")
	f.Add([]byte("task.same|hash.1\ntask.same|hash.2\n"), []byte("task.same|hash.old\n"), "close")

	f.Fuzz(func(t *testing.T, desiredRaw []byte, priorRaw []byte, policyRaw string) {
		policy, err := ParseDeletionPolicy(policyRaw)
		if err != nil {
			return
		}
		desired := parseDesiredProjections(desiredRaw)
		prior := parsePriorProjections(priorRaw)

		opsA := PlanSyncOps(desired, prior, policy)
		opsB := PlanSyncOps(parseDesiredProjections(desiredRaw), parsePriorProjections(priorRaw), policy)
		if !reflect.DeepEqual(opsA, opsB) {
			t.Fatalf("nondeterministic sync ops")
		}
		for i := 1; i < len(opsA); i++ {
			prev := opsA[i-1]
			curr := opsA[i]
			if prev.Kind > curr.Kind {
				t.Fatalf("ops not sorted by kind: %#v then %#v", prev, curr)
			}
			if prev.Kind == curr.Kind && prev.Priority > curr.Priority {
				t.Fatalf("ops not sorted by priority: %#v then %#v", prev, curr)
			}
			if prev.Kind == curr.Kind && prev.Priority == curr.Priority && prev.ID > curr.ID {
				t.Fatalf("ops not sorted by id: %#v then %#v", prev, curr)
			}
			if prev.Kind == curr.Kind && prev.Priority == curr.Priority && prev.ID == curr.ID && prev.Reason > curr.Reason {
				t.Fatalf("ops not sorted by reason: %#v then %#v", prev, curr)
			}
		}
	})
}

func parseDesiredProjections(raw []byte) []DesiredProjection {
	lines := strings.Split(string(raw), "\n")
	out := make([]DesiredProjection, 0, len(lines))
	for _, line := range lines {
		id, hash := parseProjectionLine(line)
		if id == "" && hash == "" {
			continue
		}
		out = append(out, DesiredProjection{
			ID:             id,
			ProjectionHash: hash,
			Provenance: tracker.TaskProvenance{
				NodeRef:    id,
				Path:       "PLAN.md",
				StartLine:  1,
				EndLine:    1,
				SourceHash: hash,
			},
		})
	}
	return out
}

func parsePriorProjections(raw []byte) []PriorProjection {
	lines := strings.Split(string(raw), "\n")
	out := make([]PriorProjection, 0, len(lines))
	for _, line := range lines {
		id, hash := parseProjectionLine(line)
		if id == "" && hash == "" {
			continue
		}
		out = append(out, PriorProjection{
			ID:             id,
			ProjectionHash: hash,
			Provenance: tracker.TaskProvenance{
				NodeRef:    id,
				Path:       "PLAN.md",
				StartLine:  1,
				EndLine:    1,
				SourceHash: hash,
			},
		})
	}
	return out
}

func parseProjectionLine(line string) (string, string) {
	parts := strings.SplitN(line, "|", 2)
	if len(parts) == 0 {
		return "", ""
	}
	id := strings.TrimSpace(parts[0])
	hash := ""
	if len(parts) == 2 {
		hash = strings.TrimSpace(parts[1])
	}
	return id, hash
}
