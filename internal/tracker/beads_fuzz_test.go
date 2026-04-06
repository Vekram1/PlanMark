package tracker

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func FuzzBuildProjectionPayloadDeterminism(f *testing.F) {
	f.Add("task.a", "Add migration", "now", "PLAN.md", int64(10), int64(20), "hash-123", "api.schema\napi.runtime", "cmd:go test ./...")
	f.Add("task.b", "", "", "", int64(0), int64(0), "", "", "")

	f.Fuzz(func(t *testing.T, id string, title string, horizon string, path string, start int64, end int64, sourceHash string, depsRaw string, acceptRaw string) {
		task := TaskProjection{
			ID:      strings.TrimSpace(id),
			Title:   strings.TrimSpace(title),
			Horizon: strings.TrimSpace(horizon),
			Provenance: TaskProvenance{
				NodeRef:    strings.TrimSpace(id),
				Path:       strings.TrimSpace(path),
				StartLine:  clampBeadsLine(start),
				EndLine:    clampBeadsEnd(start, end),
				SourceHash: strings.TrimSpace(sourceHash),
			},
			Dependencies: splitBeadsLines(depsRaw),
			Acceptance:   splitBeadsLines(acceptRaw),
			Steps: []TaskProjectionStep{
				{NodeRef: strings.TrimSpace(id), Title: strings.TrimSpace(title)},
			},
			Evidence: []TaskProjectionEvidence{
				{NodeRef: strings.TrimSpace(path), Kind: "note"},
			},
		}

		payloadA, errA := BuildProjectionPayload(task)
		payloadB, errB := BuildProjectionPayload(task)
		if (errA != nil) != (errB != nil) {
			t.Fatalf("nondeterministic payload error state: %v vs %v", errA, errB)
		}
		if errA != nil {
			if errA.Error() != errB.Error() {
				t.Fatalf("nondeterministic payload error text: %q vs %q", errA.Error(), errB.Error())
			}
			return
		}
		if !reflect.DeepEqual(payloadA, payloadB) {
			t.Fatalf("nondeterministic beads payload")
		}
		blobA, err := json.Marshal(payloadA)
		if err != nil {
			t.Fatalf("marshal payload A: %v", err)
		}
		blobB, err := json.Marshal(payloadB)
		if err != nil {
			t.Fatalf("marshal payload B: %v", err)
		}
		if string(blobA) != string(blobB) {
			t.Fatalf("nondeterministic payload json")
		}
	})
}

func FuzzAcceptanceDigestDeterminism(f *testing.F) {
	f.Add("cmd:go test ./...\ncmd:go test ./... -run TestOne")
	f.Add("")

	f.Fuzz(func(t *testing.T, raw string) {
		values := splitBeadsLines(raw)
		first := acceptanceDigest(values)
		second := acceptanceDigest(values)
		if first != second {
			t.Fatalf("nondeterministic acceptance digest: %q vs %q", first, second)
		}
	})
}

func splitBeadsLines(raw string) []string {
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

func clampBeadsLine(v int64) int {
	if v <= 0 {
		return 0
	}
	if v > 1_000_000 {
		return 1_000_000
	}
	return int(v)
}

func clampBeadsEnd(start int64, end int64) int {
	startLine := clampBeadsLine(start)
	endLine := clampBeadsLine(end)
	if endLine < startLine {
		return startLine
	}
	return endLine
}
