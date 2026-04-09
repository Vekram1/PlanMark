package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/tracker"
)

func (a *captureBeadsAdapter) ListCleanupCandidates(_ context.Context, planPath string, desiredIDs map[string]struct{}) ([]tracker.CleanupCandidate, error) {
	out := make([]tracker.CleanupCandidate, 0, len(a.cleanupCandidates))
	for _, candidate := range a.cleanupCandidates {
		if candidate.ExternalRef != "" {
			if _, desired := desiredIDs[candidate.ExternalRef]; desired {
				continue
			}
		}
		out = append(out, candidate)
	}
	return out, nil
}

func (a *captureBeadsAdapter) CleanupCandidate(_ context.Context, candidate tracker.CleanupCandidate) error {
	a.cleaned = append(a.cleaned, candidate)
	if a.closedIDs == nil {
		a.closedIDs = map[string]bool{}
	}
	if candidate.ExternalRef != "" {
		a.closedIDs[candidate.ExternalRef] = true
	}
	return nil
}

func TestCleanupBeadsDryRunListsCandidatesWithoutClosing(t *testing.T) {
	adapter := installCaptureBeadsAdapter(t)
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"## Context root",
		"@id pm.context.root",
		"@horizon now",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}
	adapter.cleanupCandidates = []tracker.CleanupCandidate{
		{RemoteID: "bd-1", ExternalRef: "old.task", Reason: "external_ref is missing from current plan", Title: "Old task"},
		{RemoteID: "bd-2", SourcePath: "/tmp/test/PLAN.md", Reason: "plan-derived issue points at a different plan file", Title: "Temp task"},
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"cleanup", "beads", "--plan", planPath, "--dry-run", "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}
	if len(adapter.cleaned) != 0 {
		t.Fatalf("expected dry-run to skip cleanup apply, got %#v", adapter.cleaned)
	}
	var payload struct {
		Data struct {
			DryRun           bool                       `json:"dry_run"`
			CandidatesSeen   int                        `json:"candidates_seen"`
			CandidatesClosed int                        `json:"candidates_closed"`
			Candidates       []tracker.CleanupCandidate `json:"candidates"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode cleanup output: %v output=%q", err, out.String())
	}
	if !payload.Data.DryRun || payload.Data.CandidatesSeen != 2 || payload.Data.CandidatesClosed != 0 {
		t.Fatalf("unexpected cleanup payload: %#v", payload.Data)
	}
}

func TestCleanupBeadsAppliesCandidates(t *testing.T) {
	adapter := installCaptureBeadsAdapter(t)
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"## Context root",
		"@id pm.context.root",
		"@horizon now",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}
	adapter.cleanupCandidates = []tracker.CleanupCandidate{
		{RemoteID: "bd-1", ExternalRef: "old.task", Reason: "external_ref is missing from current plan", Title: "Old task"},
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"cleanup", "beads", "--plan", planPath, "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}
	if len(adapter.cleaned) != 1 || adapter.cleaned[0].RemoteID != "bd-1" {
		t.Fatalf("expected cleanup apply to close candidate, got %#v", adapter.cleaned)
	}
}

func TestCleanupBeadsDoesNotClassifyCurrentPlanIssueAsCandidate(t *testing.T) {
	adapter := installCaptureBeadsAdapter(t)
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"## Context root",
		"@id pm.context.root",
		"@horizon now",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}
	adapter.cleanupCandidates = []tracker.CleanupCandidate{
		{RemoteID: "bd-1", ExternalRef: "pm.context.root", Reason: "external_ref is missing from current plan", Title: "Current task"},
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"cleanup", "beads", "--plan", planPath, "--dry-run", "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}
	var payload struct {
		Data struct {
			CandidatesSeen int `json:"candidates_seen"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode cleanup output: %v output=%q", err, out.String())
	}
	if payload.Data.CandidatesSeen != 0 {
		t.Fatalf("expected current plan issue to be excluded from cleanup candidates, got %#v", payload.Data)
	}
}

func TestRootHelpMentionsCleanup(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"--help"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0 for root help, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(out.String(), "cleanup         Close plan-derived Beads issues that do not belong to the current PLAN.md") {
		t.Fatalf("expected root help to mention cleanup command, got %q", out.String())
	}
}
