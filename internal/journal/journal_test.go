package journal

import (
	"strings"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/syncplanner"
)

func TestSyncOperationIDDeterministic(t *testing.T) {
	op := syncplanner.Operation{
		Kind:     syncplanner.OperationUpdate,
		ID:       "fixture.task",
		Reason:   "projection hash changed",
		Priority: 2,
	}
	a := SyncOperationID(op)
	b := SyncOperationID(op)
	if a != b {
		t.Fatalf("expected deterministic operation id")
	}
	if len(a) != 64 {
		t.Fatalf("expected sha256 hex length, got %d", len(a))
	}
}

func TestBeadsSyncRecoverAfterPartialFailure(t *testing.T) {
	stateDir := t.TempDir()
	createOp := syncplanner.Operation{Kind: syncplanner.OperationCreate, ID: "fixture.task.create", Reason: "missing", Priority: 1}
	updateOp := syncplanner.Operation{Kind: syncplanner.OperationUpdate, ID: "fixture.task.update", Reason: "changed", Priority: 2}
	noopOp := syncplanner.Operation{Kind: syncplanner.OperationNoop, ID: "fixture.task.noop", Reason: "same", Priority: 4}

	if err := AppendAttempt(stateDir, OperationAttempt{OperationID: SyncOperationID(createOp), Kind: string(createOp.Kind), ID: createOp.ID, Attempt: 1, Outcome: OutcomeSuccess}); err != nil {
		t.Fatalf("append create success: %v", err)
	}
	if err := AppendAttempt(stateDir, OperationAttempt{OperationID: SyncOperationID(updateOp), Kind: string(updateOp.Kind), ID: updateOp.ID, Attempt: 1, Outcome: OutcomeFailed, Error: "transient timeout"}); err != nil {
		t.Fatalf("append update failure: %v", err)
	}
	if err := AppendAttempt(stateDir, OperationAttempt{OperationID: SyncOperationID(noopOp), Kind: string(noopOp.Kind), ID: noopOp.ID, Attempt: 1, Outcome: OutcomeSkipped}); err != nil {
		t.Fatalf("append noop skip: %v", err)
	}

	journal, err := Load(stateDir)
	if err != nil {
		t.Fatalf("load journal: %v", err)
	}
	if journal.SchemaVersion != SyncJournalSchemaVersionV01 {
		t.Fatalf("expected schema version %q, got %q", SyncJournalSchemaVersionV01, journal.SchemaVersion)
	}
	if len(journal.Attempts) != 3 {
		t.Fatalf("expected 3 journal attempts, got %d", len(journal.Attempts))
	}

	pending := RecoverPendingMutations(journal)
	if len(pending) != 1 || pending[0] != updateOp.ID {
		t.Fatalf("expected only failed update pending, got %#v", pending)
	}

	if err := AppendAttempt(stateDir, OperationAttempt{OperationID: SyncOperationID(updateOp), Kind: string(updateOp.Kind), ID: updateOp.ID, Attempt: 2, Outcome: OutcomeSuccess}); err != nil {
		t.Fatalf("append update retry success: %v", err)
	}
	journal, err = Load(stateDir)
	if err != nil {
		t.Fatalf("reload journal: %v", err)
	}
	pending = RecoverPendingMutations(journal)
	if len(pending) != 0 {
		t.Fatalf("expected no pending operations after retry success, got %#v", pending)
	}

	if strings.TrimSpace(journal.Attempts[len(journal.Attempts)-1].OperationID) == "" {
		t.Fatalf("expected stable non-empty operation ids in journal")
	}
}

func TestRecoverPendingMutationsDeduplicatesIDs(t *testing.T) {
	stateDir := t.TempDir()
	createOp := syncplanner.Operation{Kind: syncplanner.OperationCreate, ID: "fixture.task.dup", Reason: "missing", Priority: 1}
	updateOp := syncplanner.Operation{Kind: syncplanner.OperationUpdate, ID: "fixture.task.dup", Reason: "changed", Priority: 2}

	if err := AppendAttempt(stateDir, OperationAttempt{OperationID: SyncOperationID(createOp), Kind: string(createOp.Kind), ID: createOp.ID, Attempt: 1, Outcome: OutcomeFailed, Error: "boom"}); err != nil {
		t.Fatalf("append create failure: %v", err)
	}
	if err := AppendAttempt(stateDir, OperationAttempt{OperationID: SyncOperationID(updateOp), Kind: string(updateOp.Kind), ID: updateOp.ID, Attempt: 1, Outcome: OutcomeFailed, Error: "boom"}); err != nil {
		t.Fatalf("append update failure: %v", err)
	}

	journal, err := Load(stateDir)
	if err != nil {
		t.Fatalf("load journal: %v", err)
	}
	pending := RecoverPendingMutations(journal)
	if len(pending) != 1 || pending[0] != "fixture.task.dup" {
		t.Fatalf("expected single deduplicated pending id, got %#v", pending)
	}
}
