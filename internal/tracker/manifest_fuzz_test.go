package tracker

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func FuzzBeadsBuildSyncManifestDeterminism(f *testing.F) {
	f.Add([]byte("task.a|remote.a|hash.a|PLAN.md|1|2|source.a|compile.a|runtime.a\n"))
	f.Add([]byte(" | | | |0|0| | |\n"))

	f.Fuzz(func(t *testing.T, raw []byte) {
		adapterA := NewBeadsAdapter()
		adapterB := NewBeadsAdapter()
		manifest := parseBeadsManifest(raw)
		adapterA.SeedFromSyncManifest(manifest)
		adapterB.SeedFromSyncManifest(manifest)

		first := adapterA.BuildSyncManifest()
		second := adapterB.BuildSyncManifest()
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("nondeterministic beads sync manifest")
		}
		blobA, err := json.Marshal(first)
		if err != nil {
			t.Fatalf("marshal manifest A: %v", err)
		}
		blobB, err := json.Marshal(second)
		if err != nil {
			t.Fatalf("marshal manifest B: %v", err)
		}
		if !bytes.Equal(blobA, blobB) {
			t.Fatalf("nondeterministic beads manifest json")
		}
	})
}

func FuzzLinearBuildSyncManifestDeterminism(f *testing.F) {
	f.Add([]byte("task.a|remote.a|hash.a|PLAN.md|1|2|source.a|compile.a|runtime.a\n"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, raw []byte) {
		adapterA := NewLinearAdapter()
		adapterB := NewLinearAdapter()
		manifest := parseGenericManifest(raw)
		adapterA.SeedFromSyncManifest(manifest)
		adapterB.SeedFromSyncManifest(manifest)

		first := adapterA.BuildSyncManifest()
		second := adapterB.BuildSyncManifest()
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("nondeterministic linear sync manifest")
		}
	})
}

func parseBeadsManifest(raw []byte) BeadsSyncManifest {
	lines := strings.Split(string(raw), "\n")
	entries := make([]BeadsManifestEntry, 0, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, "|")
		if len(parts) < 9 {
			continue
		}
		entries = append(entries, BeadsManifestEntry{
			ID:                  strings.TrimSpace(parts[0]),
			RemoteID:            strings.TrimSpace(parts[1]),
			ProjectionHash:      strings.TrimSpace(parts[2]),
			SourcePath:          strings.TrimSpace(parts[3]),
			SourceStartLine:     parseSmallInt(parts[4]),
			SourceEndLine:       parseSmallInt(parts[5]),
			SourceHash:          strings.TrimSpace(parts[6]),
			CompileID:           strings.TrimSpace(parts[7]),
			LastSeenRuntimeHash: strings.TrimSpace(parts[8]),
		})
	}
	return BeadsSyncManifest{
		SchemaVersion: BeadsManifestSchemaVersionV01,
		Entries:       entries,
	}
}

func parseGenericManifest(raw []byte) SyncManifest {
	lines := strings.Split(string(raw), "\n")
	entries := make([]SyncManifestEntry, 0, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, "|")
		if len(parts) < 9 {
			continue
		}
		entries = append(entries, SyncManifestEntry{
			ID:                  strings.TrimSpace(parts[0]),
			RemoteID:            strings.TrimSpace(parts[1]),
			ProjectionHash:      strings.TrimSpace(parts[2]),
			SourcePath:          strings.TrimSpace(parts[3]),
			SourceStartLine:     parseSmallInt(parts[4]),
			SourceEndLine:       parseSmallInt(parts[5]),
			SourceHash:          strings.TrimSpace(parts[6]),
			CompileID:           strings.TrimSpace(parts[7]),
			LastSeenRuntimeHash: strings.TrimSpace(parts[8]),
		})
	}
	return SyncManifest{
		SchemaVersion: SyncManifestSchemaVersionV01,
		Entries:       entries,
	}
}

func parseSmallInt(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	n := 0
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return 0
		}
		n = n*10 + int(ch-'0')
		if n > 1_000_000 {
			return 1_000_000
		}
	}
	return n
}
