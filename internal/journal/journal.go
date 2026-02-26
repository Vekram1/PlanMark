package journal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/fsio"
	"github.com/vikramoddiraju/planmark/internal/syncplanner"
)

const SyncJournalSchemaVersionV01 = "v0.1"

type Outcome string

const (
	OutcomePlanned Outcome = "planned"
	OutcomeSkipped Outcome = "skipped"
	OutcomeSuccess Outcome = "success"
	OutcomeFailed  Outcome = "failed"
)

type OperationAttempt struct {
	OperationID string  `json:"operation_id"`
	Kind        string  `json:"kind"`
	ID          string  `json:"id"`
	Attempt     int     `json:"attempt"`
	Outcome     Outcome `json:"outcome"`
	Error       string  `json:"error,omitempty"`
}

type SyncJournal struct {
	SchemaVersion string             `json:"schema_version"`
	Attempts      []OperationAttempt `json:"attempts"`
}

func SyncOperationID(op syncplanner.Operation) string {
	canonical := struct {
		Kind     string `json:"kind"`
		ID       string `json:"id"`
		Reason   string `json:"reason"`
		Priority int    `json:"priority"`
	}{
		Kind:     strings.TrimSpace(string(op.Kind)),
		ID:       strings.TrimSpace(op.ID),
		Reason:   strings.TrimSpace(op.Reason),
		Priority: op.Priority,
	}
	blob, _ := json.Marshal(canonical)
	sum := sha256.Sum256(blob)
	return hex.EncodeToString(sum[:])
}

func AppendAttempt(stateDir string, attempt OperationAttempt) error {
	journal, path, err := load(stateDir)
	if err != nil {
		return err
	}
	if strings.TrimSpace(attempt.OperationID) == "" {
		return fmt.Errorf("operation attempt requires operation_id")
	}
	if strings.TrimSpace(attempt.ID) == "" {
		return fmt.Errorf("operation attempt requires id")
	}
	if attempt.Attempt <= 0 {
		attempt.Attempt = 1
	}
	journal.Attempts = append(journal.Attempts, attempt)

	data, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal sync journal: %w", err)
	}
	data = append(data, '\n')
	if err := fsio.WriteFileAtomic(path, data, 0o644); err != nil {
		return fmt.Errorf("write sync journal: %w", err)
	}
	return nil
}

func Load(stateDir string) (SyncJournal, error) {
	journal, _, err := load(stateDir)
	return journal, err
}

func RecoverPendingMutations(j SyncJournal) []string {
	type key struct {
		opID string
		id   string
		kind string
	}
	latest := make(map[key]OperationAttempt)
	for _, attempt := range j.Attempts {
		k := key{opID: strings.TrimSpace(attempt.OperationID), id: strings.TrimSpace(attempt.ID), kind: strings.TrimSpace(attempt.Kind)}
		if k.opID == "" || k.id == "" {
			continue
		}
		prev, ok := latest[k]
		if !ok || attempt.Attempt >= prev.Attempt {
			latest[k] = attempt
		}
	}

	pendingSet := make(map[string]struct{})
	for k, attempt := range latest {
		if k.kind != string(syncplanner.OperationCreate) && k.kind != string(syncplanner.OperationUpdate) {
			continue
		}
		if attempt.Outcome == OutcomeSuccess {
			continue
		}
		pendingSet[k.id] = struct{}{}
	}
	pending := make([]string, 0, len(pendingSet))
	for id := range pendingSet {
		pending = append(pending, id)
	}
	sort.Strings(pending)
	return pending
}

func load(stateDir string) (SyncJournal, string, error) {
	path, err := pathForStateDir(stateDir)
	if err != nil {
		return SyncJournal{}, "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return SyncJournal{SchemaVersion: SyncJournalSchemaVersionV01}, path, nil
		}
		return SyncJournal{}, "", err
	}
	var journal SyncJournal
	if err := json.Unmarshal(raw, &journal); err != nil {
		return SyncJournal{}, "", fmt.Errorf("decode sync journal: %w", err)
	}
	if strings.TrimSpace(journal.SchemaVersion) == "" {
		journal.SchemaVersion = SyncJournalSchemaVersionV01
	}
	return journal, path, nil
}

func pathForStateDir(stateDir string) (string, error) {
	resolved := strings.TrimSpace(stateDir)
	if resolved == "" {
		resolved = ".planmark"
	}
	return filepath.Join(resolved, "journal", "sync-beads-journal.json"), nil
}
