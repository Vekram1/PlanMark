package build

import (
	"testing"

	"github.com/vikramoddiraju/planmark/internal/ir"
)

func TestTaskSemanticFingerprintDeterminism(t *testing.T) {
	base := ir.Task{
		ID:              "fixture.task",
		Title:           "Implement deterministic fingerprints",
		CanonicalStatus: "open",
		Horizon:         "now",
		Deps:            []string{"dep.b", "dep.a"},
		Accept:          []string{"cmd:go test ./...", "text:output includes fingerprint"},
		Sections: []ir.TaskSection{
			{Key: "details", Title: "Details", Body: []string{"first paragraph", "- bullet"}},
		},
		Steps: []ir.TaskStep{
			{NodeRef: "step.a", Title: "write code", SliceHash: "hash.a"},
			{NodeRef: "step.b", Title: "run tests", Checked: true, SliceHash: "hash.b"},
		},
		EvidenceNodeRefs: []string{"node.b", "node.a"},
	}
	reordered := ir.Task{
		ID:              " fixture.task ",
		Title:           "Implement deterministic fingerprints",
		CanonicalStatus: "OPEN",
		Horizon:         "NOW",
		Deps:            []string{"dep.a", "dep.b"},
		Accept:          []string{"text:output includes fingerprint", "cmd:go test ./..."},
		Steps: []ir.TaskStep{
			{NodeRef: "step.b", Title: "run tests", Checked: true, SliceHash: "hash.b"},
			{NodeRef: "step.a", Title: "write code", SliceHash: "hash.a"},
		},
		EvidenceNodeRefs: []string{"node.a", "node.b"},
	}

	fpA := TaskSemanticFingerprint(base)
	fpB := TaskSemanticFingerprint(reordered)
	if fpA == fpB {
		t.Fatalf("expected fingerprint change when ordered step/evidence semantics change")
	}

	changed := reordered
	changed.Steps = append(changed.Steps, ir.TaskStep{Title: "review output"})
	fpC := TaskSemanticFingerprint(changed)
	if fpB == fpC {
		t.Fatalf("expected fingerprint change when semantic steps change")
	}

	checkedChanged := base
	checkedChanged.Steps = append([]ir.TaskStep(nil), base.Steps...)
	checkedChanged.Steps[1].Checked = false
	fpD := TaskSemanticFingerprint(checkedChanged)
	if fpA == fpD {
		t.Fatalf("expected fingerprint change when step checked state changes")
	}

	provenanceChanged := base
	provenanceChanged.Steps = append([]ir.TaskStep(nil), base.Steps...)
	provenanceChanged.Steps[0].SliceHash = "hash.changed"
	fpE := TaskSemanticFingerprint(provenanceChanged)
	if fpA == fpE {
		t.Fatalf("expected fingerprint change when step provenance changes")
	}

	sectionChanged := base
	sectionChanged.Sections = append([]ir.TaskSection(nil), base.Sections...)
	sectionChanged.Sections[0].Body = []string{"first paragraph changed", "- bullet"}
	fpF := TaskSemanticFingerprint(sectionChanged)
	if fpA == fpF {
		t.Fatalf("expected fingerprint change when semantic sections change")
	}

	statusChanged := base
	statusChanged.CanonicalStatus = "done"
	fpG := TaskSemanticFingerprint(statusChanged)
	if fpA == fpG {
		t.Fatalf("expected fingerprint change when canonical task status changes")
	}

	paragraphBase := base
	paragraphBase.Sections = []ir.TaskSection{
		{Key: "details", Title: "Details", Body: []string{"First paragraph.", "", "Second paragraph."}},
	}
	fpH := TaskSemanticFingerprint(paragraphBase)

	paragraphNormalizedEquivalent := paragraphBase
	paragraphNormalizedEquivalent.Sections = []ir.TaskSection{
		{Key: "details", Title: "Details", Body: []string{"", "First paragraph.   ", "", "", "Second paragraph.", ""}},
	}
	fpI := TaskSemanticFingerprint(paragraphNormalizedEquivalent)
	if fpH != fpI {
		t.Fatalf("expected fingerprint to normalize repeated/edge blank lines consistently")
	}

	paragraphRemovedBreak := paragraphBase
	paragraphRemovedBreak.Sections = []ir.TaskSection{
		{Key: "details", Title: "Details", Body: []string{"First paragraph.", "Second paragraph."}},
	}
	fpJ := TaskSemanticFingerprint(paragraphRemovedBreak)
	if fpH == fpJ {
		t.Fatalf("expected fingerprint change when semantic paragraph break changes")
	}
}
