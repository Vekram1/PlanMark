package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestVersionJSONContainsCapabilities(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	exit := Run([]string{"version", "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}

	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing data object in payload: %v", payload)
	}
	if _, ok := data["cli_version"].(string); !ok {
		t.Fatalf("missing cli_version in payload data: %v", data)
	}
	if _, ok := data["supported_schema_versions"].([]any); !ok {
		t.Fatalf("missing supported_schema_versions in payload data: %v", data)
	}
	if _, ok := data["supported_policy_versions"].([]any); !ok {
		t.Fatalf("missing supported_policy_versions in payload data: %v", data)
	}
	if _, ok := data["exit_code_taxonomy"].([]any); !ok {
		t.Fatalf("missing exit_code_taxonomy in payload data: %v", data)
	}
	policies := data["supported_policy_versions"].([]any)
	foundDeterminism := false
	for _, item := range policies {
		p, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if name, _ := p["name"].(string); name == "determinism" {
			foundDeterminism = true
			break
		}
	}
	if !foundDeterminism {
		t.Fatalf("expected determinism policy from registry, got: %v", policies)
	}
	foundUsageError := false
	for _, item := range data["exit_code_taxonomy"].([]any) {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if name, _ := entry["name"].(string); name == "usage_error" {
			if code, _ := entry["code"].(float64); int(code) == 2 {
				foundUsageError = true
				break
			}
		}
	}
	if !foundUsageError {
		t.Fatalf("expected usage_error exit code entry in taxonomy, got: %v", data["exit_code_taxonomy"])
	}
}

func TestVersionHelpReturnsZeroAndShowsUsage(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	exit := Run([]string{"version", "--help"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0 for help, got %d", exit)
	}
	if !strings.Contains(errOut.String(), "output format: text|json") {
		t.Fatalf("expected help text to include format flag usage; stderr=%q", errOut.String())
	}
}

func TestVersionInvalidFormatReturnsUsageError(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	exit := Run([]string{"version", "--format", "yaml"}, &out, &errOut)
	if exit != 2 {
		t.Fatalf("expected exit 2 for invalid format, got %d", exit)
	}
	if !strings.Contains(errOut.String(), "invalid --format value") {
		t.Fatalf("expected invalid format error message; stderr=%q", errOut.String())
	}
}

func TestRootHelpReturnsZeroAndShowsCommandGroups(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	exit := Run([]string{"--help"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0 for root help, got %d stderr=%q", exit, errOut.String())
	}
	rendered := out.String()
	if !strings.Contains(rendered, "Canonical commands:") {
		t.Fatalf("expected root help to list canonical commands, got %q", rendered)
	}
	if !strings.Contains(rendered, "planmark <command> [flags]") {
		t.Fatalf("expected root help to mention planmark usage, got %q", rendered)
	}
	if !strings.Contains(rendered, "plan <command> [flags]") {
		t.Fatalf("expected root help to mention legacy plan usage, got %q", rendered)
	}
}

func TestHelpCommandDelegatesToSubcommandHelp(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	exit := Run([]string{"help", "version"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0 for help version, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(out.String(), "output format: text|json") {
		t.Fatalf("expected delegated version help output, got %q", out.String())
	}
}
