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
	configBody := "schema_version: v0.1\nprofiles:\n  doctor: exec\npolicies:\n  determinism: v0.1\ntracker:\n  adapter: github\n  profile: compact\nai:\n  provider: openai_compatible\n  model: gpt-4o-mini\n  base_url: http://127.0.0.1:8080/v1\n  api_key_env: PLANMARK_AI_KEY\n"
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
	if resolved.Tracker.Adapter != "github" {
		t.Fatalf("expected tracker adapter github, got %q", resolved.Tracker.Adapter)
	}
	if resolved.Tracker.Profile != "compact" {
		t.Fatalf("expected tracker profile compact, got %q", resolved.Tracker.Profile)
	}
	if resolved.AI.Provider != "openai_compatible" {
		t.Fatalf("expected ai provider openai_compatible, got %q", resolved.AI.Provider)
	}
	if resolved.AI.Model != "gpt-4o-mini" {
		t.Fatalf("expected ai model gpt-4o-mini, got %q", resolved.AI.Model)
	}
	if resolved.AI.BaseURL != "http://127.0.0.1:8080/v1" {
		t.Fatalf("expected ai base_url set, got %q", resolved.AI.BaseURL)
	}
	if resolved.AI.APIKeyEnv != "PLANMARK_AI_KEY" {
		t.Fatalf("expected ai api_key_env set, got %q", resolved.AI.APIKeyEnv)
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

func TestLoadForPlanRejectsUnknownAIKey(t *testing.T) {
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
		t.Fatalf("expected parse error for unknown ai key")
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

func TestLoadForPlanRejectsInvalidAITimeout(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	if err := os.WriteFile(planPath, []byte("- [ ] Task\n  @id task.one\n"), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}
	configPath := filepath.Join(tmp, ".planmark.yaml")
	if err := os.WriteFile(configPath, []byte("ai:\n  timeout_seconds: nope\n"), 0o644); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	if _, err := LoadForPlan(planPath); err == nil {
		t.Fatalf("expected parse error for invalid ai.timeout_seconds")
	}
}
