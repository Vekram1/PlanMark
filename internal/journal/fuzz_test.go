package journal

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/syncplanner"
)

func FuzzSyncOperationIDDeterminism(f *testing.F) {
	f.Add("create", "task.a", "missing in prior projection set", 1)
	f.Add("", "", "", 0)

	f.Fuzz(func(t *testing.T, kindRaw string, id string, reason string, priority int) {
		op := syncplanner.Operation{
			Kind:     syncplanner.OperationKind(strings.TrimSpace(kindRaw)),
			ID:       id,
			Reason:   reason,
			Priority: priority,
		}
		first := SyncOperationID(op)
		second := SyncOperationID(op)
		if first != second {
			t.Fatalf("nondeterministic sync operation id: %q vs %q", first, second)
		}
	})
}

func FuzzLoadJournalDeterminism(f *testing.F) {
	f.Add([]byte("{\"schema_version\":\"v0.1\",\"attempts\":[{\"operation_id\":\"abc\",\"kind\":\"create\",\"id\":\"task.a\",\"attempt\":1,\"outcome\":\"failed\"}]}\n"))
	f.Add([]byte("not-json"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, payload []byte) {
		tmp := t.TempDir()
		journalPath := filepath.Join(tmp, "journal", "sync-beads-journal.json")
		if err := os.MkdirAll(filepath.Dir(journalPath), 0o755); err != nil {
			t.Fatalf("mkdir journal dir: %v", err)
		}
		if err := os.WriteFile(journalPath, payload, 0o644); err != nil {
			t.Fatalf("write journal payload: %v", err)
		}

		first, errA := Load(tmp)
		second, errB := Load(tmp)
		if (errA != nil) != (errB != nil) {
			t.Fatalf("nondeterministic journal error state: %v vs %v", errA, errB)
		}
		if errA != nil {
			if errA.Error() != errB.Error() {
				t.Fatalf("nondeterministic journal error text: %q vs %q", errA.Error(), errB.Error())
			}
			return
		}
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("nondeterministic journal load result")
		}
	})
}

func FuzzRecoverPendingMutationsDeterminism(f *testing.F) {
	f.Add([]byte("op1|create|task.a|1|failed\nop2|update|task.b|2|success\n"))
	f.Add([]byte("op1|noop|task.a|1|skipped\n"))

	f.Fuzz(func(t *testing.T, raw []byte) {
		journal := SyncJournal{
			SchemaVersion: SyncJournalSchemaVersionV01,
			Attempts:      parseAttempts(raw),
		}
		first := RecoverPendingMutations(journal)
		second := RecoverPendingMutations(journal)
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("nondeterministic pending mutation recovery")
		}
		for i := 1; i < len(first); i++ {
			if first[i-1] > first[i] {
				t.Fatalf("pending ids not sorted: %#v", first)
			}
		}
	})
}

func FuzzAppendAttemptDeterminism(f *testing.F) {
	f.Add("op1", "create", "task.a", 1, "failed", "boom")
	f.Add("", "", "", 0, "", "")

	f.Fuzz(func(t *testing.T, opID string, kind string, id string, attempt int, outcomeRaw string, errText string) {
		tmp := t.TempDir()
		entry := OperationAttempt{
			OperationID: strings.TrimSpace(opID),
			Kind:        strings.TrimSpace(kind),
			ID:          strings.TrimSpace(id),
			Attempt:     attempt,
			Outcome:     Outcome(strings.TrimSpace(outcomeRaw)),
			Error:       strings.TrimSpace(errText),
		}

		errA := AppendAttempt(tmp, entry)
		errB := AppendAttempt(tmp, entry)
		if errA != nil && errB != nil {
			if errA.Error() != errB.Error() {
				t.Fatalf("nondeterministic append error text: %q vs %q", errA.Error(), errB.Error())
			}
			return
		}
		if (errA != nil) != (errB != nil) {
			t.Fatalf("nondeterministic append error state: %v vs %v", errA, errB)
		}
	})
}

func parseAttempts(raw []byte) []OperationAttempt {
	lines := strings.Split(string(raw), "\n")
	out := make([]OperationAttempt, 0, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, "|")
		if len(parts) < 5 {
			continue
		}
		out = append(out, OperationAttempt{
			OperationID: strings.TrimSpace(parts[0]),
			Kind:        strings.TrimSpace(parts[1]),
			ID:          strings.TrimSpace(parts[2]),
			Attempt:     clampAttempt(parts[3]),
			Outcome:     Outcome(strings.TrimSpace(parts[4])),
		})
	}
	return out
}

func clampAttempt(raw string) int {
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
