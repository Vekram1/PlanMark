package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

const (
	CLIVersion    = "0.1.0-dev"
	SchemaVersion = "v0.1"
)

type supportedPolicy struct {
	Name     string   `json:"name"`
	Versions []string `json:"versions"`
}

type versionData struct {
	CLIVersion              string            `json:"cli_version"`
	SupportedSchemaVersions []string          `json:"supported_schema_versions"`
	SupportedPolicyVersions []supportedPolicy `json:"supported_policy_versions"`
}

type versionEnvelope struct {
	SchemaVersion string      `json:"schema_version"`
	Command       string      `json:"command"`
	Status        string      `json:"status"`
	Data          versionData `json:"data"`
}

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: plan <command> [flags]")
		return 2
	}

	switch args[0] {
	case "version":
		return runVersion(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
		return 2
	}
}

func runVersion(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "text", "output format: text|json")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
		}
		return 2
	}

	payload := versionEnvelope{
		SchemaVersion: SchemaVersion,
		Command:       "version",
		Status:        "ok",
		Data: versionData{
			CLIVersion:              CLIVersion,
			SupportedSchemaVersions: []string{"planmark-v0.2", "ir-v0.2"},
			SupportedPolicyVersions: []supportedPolicy{
				{Name: "determinism", Versions: []string{"v0.1"}},
				{Name: "semantic_derivation", Versions: []string{"v0.1"}},
				{Name: "change_detection", Versions: []string{"v0.1"}},
				{Name: "tracker_reconcile", Versions: []string{"v0.1"}},
			},
		},
	}

	switch *format {
	case "json":
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(payload); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return 1
		}
		return 0
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
		return 0
	default:
		fmt.Fprintf(stderr, "invalid --format value: %s\n", *format)
		return 2
	}
}
