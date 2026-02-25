package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/vikramoddiraju/planmark/internal/compile"
	"github.com/vikramoddiraju/planmark/internal/diag"
	"github.com/vikramoddiraju/planmark/internal/doctor"
	"github.com/vikramoddiraju/planmark/internal/protocol"
)

func runDoctor(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	planPath := fs.String("plan", "", "path to PLAN markdown file")
	profile := fs.String("profile", "loose", "strictness profile: loose|build|exec")
	format := fs.String("format", "text", "output format: text|json")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return protocol.ExitSuccess
		}
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitUsageError
	}

	if *planPath == "" {
		fmt.Fprintln(stderr, "missing --plan")
		return protocol.ExitUsageError
	}

	content, err := os.ReadFile(*planPath)
	if err != nil {
		fmt.Fprintf(stderr, "read plan: %v\n", err)
		return protocol.ExitInternalError
	}
	compiled, err := compile.CompilePlan(*planPath, content, compile.NewParser(nil))
	if err != nil {
		fmt.Fprintf(stderr, "compile plan: %v\n", err)
		return protocol.ExitInternalError
	}

	result, err := doctor.Run(compiled, *profile)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitUsageError
	}

	errorCount, warningCount := countBySeverity(result.Diagnostics)

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
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(result); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return protocol.ExitInternalError
		}
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
