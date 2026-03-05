package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCreatesPlanmarkLayoutAndTemplates(t *testing.T) {
	tmp := t.TempDir()
	var out bytes.Buffer
	var errOut bytes.Buffer

	exit := Run([]string{"init", "--dir", tmp, "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}

	required := []string{
		filepath.Join(tmp, ".planmark"),
		filepath.Join(tmp, ".planmark", "build"),
		filepath.Join(tmp, ".planmark", "sync"),
		filepath.Join(tmp, ".planmark", "cache", "context"),
		filepath.Join(tmp, ".planmark", "cas", "sha256"),
		filepath.Join(tmp, ".planmark", "journal", "sync"),
		filepath.Join(tmp, ".planmark", "locks"),
		filepath.Join(tmp, ".planmark", "state_version.json"),
		filepath.Join(tmp, "PLAN.md"),
		filepath.Join(tmp, ".planmark.yaml"),
		filepath.Join(tmp, "AGENTS.md"),
	}
	for _, path := range required {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected created path %q: %v", path, err)
		}
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected envelope data object, got: %v", payload)
	}
	if got, _ := data["state_dir"].(string); !strings.HasSuffix(got, ".planmark") {
		t.Fatalf("expected state_dir to end with .planmark, got %q", got)
	}
	agentsRaw, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if !strings.Contains(string(agentsRaw), agentsGuideStart) || !strings.Contains(string(agentsRaw), agentsGuideEnd) {
		t.Fatalf("expected AGENTS.md to contain managed guide markers")
	}
}

func TestInitDoesNotOverwriteExistingPlanOrConfig(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	cfgPath := filepath.Join(tmp, ".planmark.yaml")
	planContent := "existing plan\n"
	cfgContent := "profile: exec\n"
	if err := os.WriteFile(planPath, []byte(planContent), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"init", "--dir", tmp}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}

	gotPlan, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("read plan fixture: %v", err)
	}
	gotCfg, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config fixture: %v", err)
	}
	if string(gotPlan) != planContent {
		t.Fatalf("plan was overwritten: got %q", string(gotPlan))
	}
	if string(gotCfg) != cfgContent {
		t.Fatalf("config was overwritten: got %q", string(gotCfg))
	}
}

func TestInitNoConfigNoPlanTemplate(t *testing.T) {
	tmp := t.TempDir()
	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"init", "--dir", tmp, "--no-config", "--no-plan-template", "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}

	if _, err := os.Stat(filepath.Join(tmp, "PLAN.md")); !os.IsNotExist(err) {
		t.Fatalf("expected PLAN.md to be absent; err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, ".planmark.yaml")); !os.IsNotExist(err) {
		t.Fatalf("expected .planmark.yaml to be absent; err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, ".planmark", "state_version.json")); err != nil {
		t.Fatalf("expected state_version.json to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "AGENTS.md")); err != nil {
		t.Fatalf("expected AGENTS.md to exist: %v", err)
	}
}

func TestInitUpdatesManagedAgentsBlockWithoutClobberingOtherContent(t *testing.T) {
	tmp := t.TempDir()
	agentsPath := filepath.Join(tmp, "AGENTS.md")
	initial := strings.Join([]string{
		"# Team Rules",
		"",
		"Custom instructions above.",
		"",
		agentsGuideStart,
		"old block",
		agentsGuideEnd,
		"",
		"Custom instructions below.",
		"",
	}, "\n")
	if err := os.WriteFile(agentsPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("write AGENTS fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"init", "--dir", tmp}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}

	got, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read AGENTS output: %v", err)
	}
	text := string(got)
	if !strings.Contains(text, "Custom instructions above.") || !strings.Contains(text, "Custom instructions below.") {
		t.Fatalf("expected non-managed AGENTS content to remain")
	}
	if strings.Contains(text, "old block") {
		t.Fatalf("expected managed block to be replaced")
	}
	if strings.Count(text, agentsGuideStart) != 1 || strings.Count(text, agentsGuideEnd) != 1 {
		t.Fatalf("expected exactly one managed block")
	}
}

func TestInitInvalidFormatReturnsUsageError(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"init", "--format", "yaml"}, &out, &errOut)
	if exit != 2 {
		t.Fatalf("expected exit 2, got %d", exit)
	}
	if !strings.Contains(errOut.String(), "invalid --format value") {
		t.Fatalf("expected invalid format error message; stderr=%q", errOut.String())
	}
}
