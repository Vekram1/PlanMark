package tracker

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func FuzzTaskProjectionHashDeterminism(f *testing.F) {
	f.Add("task.a", "Add migration", "now", "PLAN.md", int64(10), int64(20), "api.schema\napi.runtime", "cmd:go test ./...", "why=Preserve rollout safety")
	f.Add("task.b", "", "", "", int64(0), int64(0), "", "", "")

	f.Fuzz(func(t *testing.T, id string, title string, horizon string, path string, start int64, end int64, depsRaw string, acceptRaw string, sectionsRaw string) {
		task := fuzzTaskProjection(id, title, horizon, path, start, end, depsRaw, acceptRaw, sectionsRaw)
		hashA, errA := TaskProjectionHash(task)
		hashB, errB := TaskProjectionHash(task)
		if (errA != nil) != (errB != nil) {
			t.Fatalf("nondeterministic projection hash error state: %v vs %v", errA, errB)
		}
		if errA != nil {
			if errA.Error() != errB.Error() {
				t.Fatalf("nondeterministic projection hash error text: %q vs %q", errA.Error(), errB.Error())
			}
			return
		}
		if hashA != hashB {
			t.Fatalf("nondeterministic projection hash: %q vs %q", hashA, hashB)
		}
	})
}

func FuzzRenderTaskDeterminism(f *testing.F) {
	f.Add("task.a", "Add migration", "now", "PLAN.md", int64(10), int64(20), "api.schema", "cmd:go test ./...", "why=Preserve rollout safety", "default", true, "rendered")
	f.Add("task.b", "", "", "", int64(0), int64(0), "", "", "", "compact", true, "native")

	f.Fuzz(func(t *testing.T, id string, title string, horizon string, path string, start int64, end int64, depsRaw string, acceptRaw string, sectionsRaw string, profileRaw string, titleCap bool, stepsCapRaw string) {
		task := fuzzTaskProjection(id, title, horizon, path, start, end, depsRaw, acceptRaw, sectionsRaw)
		caps := NewBeadsAdapter().Capabilities()
		caps.Title = titleCap
		caps.Steps = fuzzCapabilitySupport(stepsCapRaw)
		profile := RenderProfile(profileRaw)

		renderedA, errA := RenderTask(task, caps, profile)
		renderedB, errB := RenderTask(task, caps, profile)
		if (errA != nil) != (errB != nil) {
			t.Fatalf("nondeterministic render error state: %v vs %v", errA, errB)
		}
		if errA != nil {
			if errA.Error() != errB.Error() {
				t.Fatalf("nondeterministic render error text: %q vs %q", errA.Error(), errB.Error())
			}
			return
		}
		if !reflect.DeepEqual(renderedA, renderedB) {
			t.Fatalf("nondeterministic rendered task")
		}

		bodyJSONA, err := json.Marshal(renderedA.Body)
		if err != nil {
			t.Fatalf("marshal body A: %v", err)
		}
		bodyJSONB, err := json.Marshal(renderedB.Body)
		if err != nil {
			t.Fatalf("marshal body B: %v", err)
		}
		if string(bodyJSONA) != string(bodyJSONB) {
			t.Fatalf("nondeterministic rendered body json")
		}
	})
}

func fuzzTaskProjection(id string, title string, horizon string, path string, start int64, end int64, depsRaw string, acceptRaw string, sectionsRaw string) TaskProjection {
	if end < start {
		end = start
	}
	return TaskProjection{
		ID:      strings.TrimSpace(id),
		Title:   strings.TrimSpace(title),
		Horizon: strings.TrimSpace(horizon),
		Anchor:  strings.TrimSpace(path),
		Provenance: TaskProvenance{
			NodeRef:    strings.TrimSpace(id),
			Path:       strings.TrimSpace(path),
			StartLine:  clampPositive(start),
			EndLine:    clampPositive(end),
			SourceHash: strings.TrimSpace(title),
			CompileID:  strings.TrimSpace(horizon),
		},
		Dependencies: splitNonEmptyLines(depsRaw),
		Acceptance:   splitNonEmptyLines(acceptRaw),
		Steps: []TaskProjectionStep{
			{NodeRef: strings.TrimSpace(id), Title: strings.TrimSpace(title)},
		},
		Evidence: []TaskProjectionEvidence{
			{NodeRef: strings.TrimSpace(path), Kind: "note"},
		},
		Sections: fuzzSections(sectionsRaw),
	}
}

func fuzzSections(raw string) []TaskProjectionSection {
	lines := splitNonEmptyLines(raw)
	out := make([]TaskProjectionSection, 0, len(lines))
	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		body := ""
		if len(parts) == 2 {
			body = strings.TrimSpace(parts[1])
		}
		out = append(out, TaskProjectionSection{
			Key:   key,
			Title: key,
			Body:  splitNonEmptyLines(body),
		})
	}
	return out
}

func splitNonEmptyLines(raw string) []string {
	parts := strings.Split(raw, "\n")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func clampPositive(v int64) int {
	if v <= 0 {
		return 0
	}
	if v > 1_000_000 {
		return 1_000_000
	}
	return int(v)
}

func fuzzCapabilitySupport(raw string) CapabilitySupport {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(CapabilityNative):
		return CapabilityNative
	case string(CapabilityUnsupported):
		return CapabilityUnsupported
	default:
		return CapabilityRendered
	}
}
