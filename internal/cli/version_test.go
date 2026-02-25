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
