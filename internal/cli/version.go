package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/policy"
	"github.com/vikramoddiraju/planmark/internal/protocol"
)

const (
	CLIVersion = "0.1.4"
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
		renderRootHelp(stdout)
		return protocol.ExitSuccess
	}

	switch normalizeCommandToken(args[0]) {
	case "help":
		return runHelp(args[1:], stdout, stderr)
	case "version":
		return runVersion(args[1:], stdout, stderr)
	case "update":
		return runUpdate(args[1:], stdout, stderr)
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "compile":
		return runCompile(args[1:], stdout, stderr)
	case "doctor":
		return runDoctor(args[1:], stdout, stderr)
	case "context":
		return runContext(args[1:], stdout, stderr)
	case "open":
		return runOpen(args[1:], stdout, stderr)
	case "handoff":
		return runHandoff(args[1:], stdout, stderr)
	case "explain":
		return runExplain(args[1:], stdout, stderr)
	case "sync":
		return runSync(args[1:], stdout, stderr)
	case "changes":
		return runChanges(args[1:], stdout, stderr)
	case "propose-change":
		return runProposeChange(args[1:], stdout, stderr)
	case "apply-change":
		return runApplyChange(args[1:], stdout, stderr)
	case "pack":
		return runPack(args[1:], stdout, stderr)
	case "query":
		return runQuery(args[1:], stdout, stderr)
	case "id":
		return runID(args[1:], stdout, stderr)
	case "verify-accept":
		return runVerifyAccept(args[1:], stdout, stderr)
	case "ai":
		return runAI(args[1:], stdout, stderr)
	default:
		if isHelpToken(args[0]) {
			renderRootHelp(stdout)
			return protocol.ExitSuccess
		}
		fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
		fmt.Fprintln(stderr, "")
		renderRootHelp(stderr)
		return protocol.ExitUsageError
	}
}

func runHelp(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		renderRootHelp(stdout)
		return protocol.ExitSuccess
	}

	target := normalizeCommandToken(args[0])
	switch target {
	case "version":
		return runVersion([]string{"--help"}, stdout, stdout)
	case "update":
		return runUpdate([]string{"--help"}, stdout, stdout)
	case "init":
		return runInit([]string{"--help"}, stdout, stdout)
	case "compile":
		return runCompile([]string{"--help"}, stdout, stdout)
	case "doctor":
		return runDoctor([]string{"--help"}, stdout, stdout)
	case "context":
		return runContext([]string{"--help"}, stdout, stdout)
	case "open":
		return runOpen([]string{"--help"}, stdout, stdout)
	case "handoff":
		return runHandoff([]string{"--help"}, stdout, stdout)
	case "explain":
		return runExplain([]string{"--help"}, stdout, stdout)
	case "sync":
		return runSync([]string{"--help"}, stdout, stdout)
	case "changes":
		return runChanges([]string{"--help"}, stdout, stdout)
	case "propose-change":
		return runProposeChange([]string{"--help"}, stdout, stdout)
	case "apply-change":
		return runApplyChange([]string{"--help"}, stdout, stdout)
	case "pack":
		return runPack([]string{"--help"}, stdout, stdout)
	case "query":
		return runQuery([]string{"--help"}, stdout, stdout)
	case "id":
		return runID([]string{"--help"}, stdout, stdout)
	case "verify-accept":
		return runVerifyAccept([]string{"--help"}, stdout, stdout)
	case "ai":
		return runAI([]string{"--help"}, stdout, stdout)
	default:
		fmt.Fprintf(stderr, "unknown help topic: %s\n", args[0])
		fmt.Fprintln(stderr, "")
		renderRootHelp(stderr)
		return protocol.ExitUsageError
	}
}

func renderRootHelp(w io.Writer) {
	lines := []string{
		"PlanMark turns PLAN.md into deterministic planning artifacts.",
		"",
		"Usage:",
		"  planmark <command> [flags]",
		"  plan <command> [flags]",
		"",
		"Canonical commands:",
		"  version         Show CLI, schema, and policy support information",
		"  update          Check for and install the latest released PlanMark build",
		"  init            Initialize repo-local PlanMark state and starter files",
		"  compile         Compile PLAN.md into deterministic IR JSON",
		"  doctor          Check plan validity and readiness under a strictness profile",
		"  query           List tasks with optional readiness and horizon filters",
		"  context         Build a deterministic task context packet",
		"  open            Show exact source scope for a task or node reference",
		"  explain         Explain why a task looks the way it does",
		"  handoff         Build an agent-oriented handoff packet",
		"  changes         Compare current plan state against prior compile or git ref",
		"  pack            Export plan and packets into a portable pack",
		"  sync            Project tasks into a tracker without making it canonical",
		"  propose-change  Produce a deterministic plan delta proposal",
		"  apply-change    Apply a deterministic plan delta",
		"  id              Generate a deterministic task id from a title",
		"  verify-accept   Run an explicit @accept command and record a receipt",
		"",
		"Assistive commands:",
		"  ai              Non-canonical AI helpers for drafting and suggestion flows",
		"",
		"Help:",
		"  planmark help <command>",
		"  plan help <command>",
		"  planmark --help",
		"  plan --help",
		"",
		"Notes:",
		"  PLAN.md remains canonical.",
		"  compile/doctor/context/open/explain/handoff/sync are intended to remain deterministic and offline-safe.",
	}
	for _, line := range lines {
		fmt.Fprintln(w, line)
	}
}

func normalizeCommandToken(raw string) string {
	return strings.TrimSpace(strings.ToLower(raw))
}

func isHelpToken(raw string) bool {
	switch normalizeCommandToken(filepath.Base(raw)) {
	case "-h", "--help":
		return true
	default:
		return false
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
