package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/compile"
	"github.com/vikramoddiraju/planmark/internal/config"
	"github.com/vikramoddiraju/planmark/internal/diag"
	"github.com/vikramoddiraju/planmark/internal/doctor"
	"github.com/vikramoddiraju/planmark/internal/fsio"
	"github.com/vikramoddiraju/planmark/internal/protocol"
)

func runDoctor(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	planPath := fs.String("plan", "", "path to `plan-file` markdown file")
	profileFlag := hasLongFlag(args, "profile")
	profile := fs.String("profile", "", "strictness profile: loose|build|exec")
	format := fs.String("format", "text", "output format: text|rich|json")
	fixOut := fs.String("fix-out", "", "write deterministic repair suggestions to `delta-json-file` (does not modify the plan file)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return protocol.ExitSuccess
		}
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitUsageError
	}
	if strings.HasPrefix(strings.TrimSpace(*fixOut), "-") {
		fmt.Fprintln(stderr, "invalid --fix-out value: expected delta JSON output path")
		return protocol.ExitUsageError
	}

	if *planPath == "" {
		fmt.Fprintln(stderr, "missing --plan")
		return protocol.ExitUsageError
	}

	repoConfig, err := config.LoadForPlan(*planPath)
	if err != nil {
		fmt.Fprintf(stderr, "load repo config: %v\n", err)
		return protocol.ExitUsageError
	}

	selectedProfile := strings.TrimSpace(*profile)
	if selectedProfile == "" && !profileFlag {
		selectedProfile = strings.TrimSpace(repoConfig.Profile)
	}
	if selectedProfile == "" {
		selectedProfile = "loose"
	}

	normalizedProfile, err := doctor.NormalizeProfile(selectedProfile)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitUsageError
	}

	content, err := os.ReadFile(*planPath)
	if err != nil {
		fmt.Fprintf(stderr, "read plan: %v\n", err)
		return protocol.ExitInternalError
	}
	compiled, err := compile.CompilePlan(*planPath, content, compile.NewParser(nil))
	if err != nil {
		if d, ok := doctor.DiagnosticFromCompileError(err); ok {
			result := doctor.Result{
				Profile:     normalizedProfile,
				ParsedNodes: 0,
				ParsedTasks: 0,
				Diagnostics: []diag.Diagnostic{d},
			}
			switch *format {
			case "text":
				fmt.Fprintf(stdout, "profile: %s\n", result.Profile)
				fmt.Fprintf(stdout, "parsed_nodes: %d\n", result.ParsedNodes)
				fmt.Fprintf(stdout, "parsed_tasks: %d\n", result.ParsedTasks)
				fmt.Fprintf(stdout, "diagnostics: total=1 errors=1 warnings=0\n")
				fmt.Fprintln(stdout, doctor.FormatDiagnosticsText(result.Diagnostics))
			case "json":
				payload := protocol.Envelope[doctor.Result]{
					SchemaVersion: protocol.SchemaVersionV01,
					ToolVersion:   CLIVersion,
					Command:       "doctor",
					Status:        "validation_failed",
					Data:          result,
				}
				enc := json.NewEncoder(stdout)
				enc.SetEscapeHTML(false)
				if err := enc.Encode(payload); err != nil {
					fmt.Fprintln(stderr, err.Error())
					return protocol.ExitInternalError
				}
			case "rich":
				fmt.Fprintf(stdout, "profile: %s\n", result.Profile)
				fmt.Fprintf(stdout, "parsed_nodes: %d\n", result.ParsedNodes)
				fmt.Fprintf(stdout, "parsed_tasks: %d\n", result.ParsedTasks)
				fmt.Fprintln(stdout, doctor.FormatDiagnosticsRich(result.Diagnostics))
			default:
				fmt.Fprintf(stderr, "invalid --format value: %s\n", *format)
				return protocol.ExitUsageError
			}
			return protocol.ExitValidationFailed
		}
		fmt.Fprintf(stderr, "compile plan: %v\n", err)
		return protocol.ExitInternalError
	}

	result, err := doctor.Run(compiled, normalizedProfile)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitUsageError
	}

	errorCount, warningCount := countBySeverity(result.Diagnostics)

	if strings.TrimSpace(*fixOut) != "" {
		fix := doctor.BuildFixOut(compiled, normalizedProfile)
		if err := writeJSONFile(*fixOut, fix); err != nil {
			fmt.Fprintf(stderr, "write --fix-out: %v\n", err)
			return protocol.ExitInternalError
		}
	}

	switch *format {
	case "text":
		fmt.Fprintf(stdout, "profile: %s\n", result.Profile)
		fmt.Fprintf(stdout, "parsed_nodes: %d\n", result.ParsedNodes)
		fmt.Fprintf(stdout, "parsed_tasks: %d\n", result.ParsedTasks)
		fmt.Fprintf(stdout, "diagnostics: total=%d errors=%d warnings=%d\n", len(result.Diagnostics), errorCount, warningCount)
		if len(result.Diagnostics) > 0 {
			fmt.Fprintln(stdout, doctor.FormatDiagnosticsText(result.Diagnostics))
		}
	case "json":
		status := "ok"
		if errorCount > 0 {
			status = "validation_failed"
		}
		payload := protocol.Envelope[doctor.Result]{
			SchemaVersion: protocol.SchemaVersionV01,
			ToolVersion:   CLIVersion,
			Command:       "doctor",
			Status:        status,
			Data:          result,
		}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(payload); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return protocol.ExitInternalError
		}
	case "rich":
		fmt.Fprintf(stdout, "profile: %s\n", result.Profile)
		fmt.Fprintf(stdout, "parsed_nodes: %d\n", result.ParsedNodes)
		fmt.Fprintf(stdout, "parsed_tasks: %d\n", result.ParsedTasks)
		fmt.Fprintln(stdout, doctor.FormatDiagnosticsRich(result.Diagnostics))
	default:
		fmt.Fprintf(stderr, "invalid --format value: %s\n", *format)
		return protocol.ExitUsageError
	}

	if errorCount > 0 {
		return protocol.ExitValidationFailed
	}
	return protocol.ExitSuccess
}

func countBySeverity(diagnostics []diag.Diagnostic) (errors int, warnings int) {
	for _, d := range diagnostics {
		switch d.Severity {
		case diag.SeverityError:
			errors++
		case diag.SeverityWarning:
			warnings++
		}
	}
	return errors, warnings
}

func writeJSONFile(path string, value any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		return err
	}
	return fsio.WriteFileAtomic(path, buf.Bytes(), 0o644)
}
