package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadForPlanNotFound(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	if err := os.WriteFile(planPath, []byte("- [ ] Task\n  @id task.one\n"), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	resolved, err := LoadForPlan(planPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if resolved.Found {
		t.Fatalf("expected no config to be found")
	}
	if resolved.Hash != "" {
		t.Fatalf("expected empty hash when no config found, got %q", resolved.Hash)
	}
}

func TestLoadForPlanUsesNearestConfigAndProfilesDoctor(t *testing.T) {
	tmp := t.TempDir()
	repoRoot := filepath.Join(tmp, "repo")
	nested := filepath.Join(repoRoot, "nested", "deeper")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	configPath := filepath.Join(repoRoot, ".planmark.yaml")
	configBody := "schema_version: v0.1\nprofiles:\n  doctor: exec\npolicies:\n  determinism: v0.1\ntracker:\n  adapter: linear\n  profile: compact\n"
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	planPath := filepath.Join(nested, "PLAN.md")
	if err := os.WriteFile(planPath, []byte("- [ ] Task\n  @id task.one\n"), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	resolved, err := LoadForPlan(planPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !resolved.Found {
		t.Fatalf("expected config found")
	}
	if resolved.Path != configPath {
		t.Fatalf("expected config path %q, got %q", configPath, resolved.Path)
	}
	if resolved.Profile != "exec" {
		t.Fatalf("expected profile exec, got %q", resolved.Profile)
	}
	if resolved.Hash == "" {
		t.Fatalf("expected non-empty hash")
	}
	if resolved.Tracker.Adapter != "linear" {
		t.Fatalf("expected tracker adapter linear, got %q", resolved.Tracker.Adapter)
	}
	if resolved.Tracker.Profile != "compact" {
		t.Fatalf("expected tracker profile compact, got %q", resolved.Tracker.Profile)
	}
}

func TestLoadForPlanRejectsUnknownTopLevelKey(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	if err := os.WriteFile(planPath, []byte("- [ ] Task\n  @id task.one\n"), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}
	configPath := filepath.Join(tmp, ".planmark.yaml")
	if err := os.WriteFile(configPath, []byte("unknown: yes\n"), 0o644); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	if _, err := LoadForPlan(planPath); err == nil {
		t.Fatalf("expected parse error for unknown top-level key")
	}
}

func TestLoadForPlanRejectsRemovedAITopLevelKey(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	if err := os.WriteFile(planPath, []byte("- [ ] Task\n  @id task.one\n"), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}
	configPath := filepath.Join(tmp, ".planmark.yaml")
	if err := os.WriteFile(configPath, []byte("ai:\n  unknown_field: x\n"), 0o644); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	if _, err := LoadForPlan(planPath); err == nil {
		t.Fatalf("expected parse error for removed ai top-level key")
	}
}

func TestLoadForPlanRejectsUnknownTrackerKey(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	if err := os.WriteFile(planPath, []byte("- [ ] Task\n  @id task.one\n"), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}
	configPath := filepath.Join(tmp, ".planmark.yaml")
	if err := os.WriteFile(configPath, []byte("tracker:\n  unsupported: x\n"), 0o644); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	if _, err := LoadForPlan(planPath); err == nil {
		t.Fatalf("expected parse error for unknown tracker key")
	}
}

func TestLoadForPlanRejectsUnsupportedTrackerAdapter(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	if err := os.WriteFile(planPath, []byte("- [ ] Task\n  @id task.one\n"), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}
	configPath := filepath.Join(tmp, ".planmark.yaml")
	if err := os.WriteFile(configPath, []byte("tracker:\n  adapter: github\n"), 0o644); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	if _, err := LoadForPlan(planPath); err == nil {
		t.Fatalf("expected parse error for unsupported tracker adapter")
	}
}
