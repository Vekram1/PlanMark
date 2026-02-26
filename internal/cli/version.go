package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/policy"
	"github.com/vikramoddiraju/planmark/internal/protocol"
)

const (
	CLIVersion = "0.1.0-dev"
)

type supportedPolicy struct {
	Name     string   `json:"name"`
	Versions []string `json:"versions"`
}

type versionData struct {
	CLIVersion              string            `json:"cli_version"`
	SupportedSchemaVersions []string          `json:"supported_schema_versions"`
	SupportedPolicyVersions []supportedPolicy `json:"supported_policy_versions"`
	ExitCodeTaxonomy        []exitCodeInfo    `json:"exit_code_taxonomy"`
}

type exitCodeInfo struct {
	Name    string `json:"name"`
	Code    int    `json:"code"`
	Meaning string `json:"meaning"`
}

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: plan <command> [flags]")
		return protocol.ExitUsageError
	}

	switch args[0] {
	case "version":
		return runVersion(args[1:], stdout, stderr)
	case "compile":
		return runCompile(args[1:], stdout, stderr)
	case "doctor":
		return runDoctor(args[1:], stdout, stderr)
	case "context":
		return runContext(args[1:], stdout, stderr)
	case "open":
		return runOpen(args[1:], stdout, stderr)
	case "explain":
		return runExplain(args[1:], stdout, stderr)
	case "sync":
		return runSync(args[1:], stdout, stderr)
	case "changes":
		return runChanges(args[1:], stdout, stderr)
	case "propose-change":
		return runProposeChange(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
		return protocol.ExitUsageError
	}
}

func runVersion(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "text", "output format: text|json")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return protocol.ExitSuccess
		}
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
		}
		return protocol.ExitUsageError
	}

	payload := protocol.Envelope[versionData]{
		SchemaVersion: protocol.SchemaVersionV01,
		ToolVersion:   CLIVersion,
		Command:       "version",
		Status:        "ok",
		Data: versionData{
			CLIVersion:              CLIVersion,
			SupportedSchemaVersions: []string{"planmark-v0.2", "ir-v0.2"},
			SupportedPolicyVersions: supportedPoliciesFromRegistry(policy.NewRegistry()),
			ExitCodeTaxonomy:        exitCodeTaxonomy(),
		},
	}

	switch *format {
	case "json":
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(payload); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return protocol.ExitInternalError
		}
		return protocol.ExitSuccess
	case "text":
		fmt.Fprintf(stdout, "plan %s\n", payload.Data.CLIVersion)
		fmt.Fprintln(stdout, "supported schema versions:")
		for _, v := range payload.Data.SupportedSchemaVersions {
			fmt.Fprintf(stdout, "- %s\n", v)
		}
		fmt.Fprintln(stdout, "supported policy versions:")
		for _, p := range payload.Data.SupportedPolicyVersions {
			fmt.Fprintf(stdout, "- %s: %s\n", p.Name, strings.Join(p.Versions, ","))
		}
		fmt.Fprintln(stdout, "exit code taxonomy:")
		for _, e := range payload.Data.ExitCodeTaxonomy {
			fmt.Fprintf(stdout, "- %d (%s): %s\n", e.Code, e.Name, e.Meaning)
		}
		return protocol.ExitSuccess
	default:
		fmt.Fprintf(stderr, "invalid --format value: %s\n", *format)
		return protocol.ExitUsageError
	}
}

func exitCodeTaxonomy() []exitCodeInfo {
	return []exitCodeInfo{
		{Name: "success", Code: protocol.ExitSuccess, Meaning: "Command completed successfully."},
		{Name: "validation_failed", Code: protocol.ExitValidationFailed, Meaning: "Command completed with validation/readiness errors."},
		{Name: "usage_error", Code: protocol.ExitUsageError, Meaning: "Invalid command usage or flag values."},
		{Name: "internal_error", Code: protocol.ExitInternalError, Meaning: "Unexpected internal failure while producing output."},
	}
}

func supportedPoliciesFromRegistry(reg *policy.Registry) []supportedPolicy {
	supported := reg.SupportedVersions()
	order := []policy.Kind{
		policy.KindDeterminism,
		policy.KindSemanticDerivation,
		policy.KindChangeDetection,
		policy.KindTrackerReconcile,
	}
	out := make([]supportedPolicy, 0, len(order))
	for _, kind := range order {
		versions := append([]string(nil), supported[kind]...)
		out = append(out, supportedPolicy{
			Name:     string(kind),
			Versions: versions,
		})
	}
	return out
}
