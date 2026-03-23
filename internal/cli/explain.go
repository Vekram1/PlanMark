package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/compile"
	contextpkg "github.com/vikramoddiraju/planmark/internal/context"
	"github.com/vikramoddiraju/planmark/internal/protocol"
)

func runExplain(args []string, stdout io.Writer, stderr io.Writer) int {
	positionalID := ""
	filteredArgs := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--plan=") || strings.HasPrefix(arg, "--format=") {
			filteredArgs = append(filteredArgs, arg)
			continue
		}
		if arg == "--plan" || arg == "--format" {
			filteredArgs = append(filteredArgs, arg)
			if i+1 < len(args) {
				i++
				filteredArgs = append(filteredArgs, args[i])
			}
			continue
		}
		if strings.HasPrefix(arg, "-") {
			filteredArgs = append(filteredArgs, arg)
			continue
		}
		if positionalID == "" {
			positionalID = arg
			continue
		}
		filteredArgs = append(filteredArgs, arg)
	}

	fs := flag.NewFlagSet("explain", flag.ContinueOnError)
	fs.SetOutput(stderr)
	planPath := fs.String("plan", "", "path to PLAN markdown file")
	format := fs.String("format", "text", "output format: text|rich|json")
	if err := fs.Parse(filteredArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return protocol.ExitSuccess
		}
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitUsageError
	}

	remaining := fs.Args()
	if len(remaining) > 1 {
		fmt.Fprintln(stderr, "usage: plan explain <id> --plan <path> [--format text|rich|json]")
		return protocol.ExitUsageError
	}
	if len(remaining) == 1 {
		if positionalID != "" {
			fmt.Fprintln(stderr, "too many positional arguments for explain")
			return protocol.ExitUsageError
		}
		positionalID = remaining[0]
	}
	if strings.TrimSpace(positionalID) == "" {
		fmt.Fprintln(stderr, "usage: plan explain <id> --plan <path> [--format text|rich|json]")
		return protocol.ExitUsageError
	}
	if *planPath == "" {
		fmt.Fprintln(stderr, "missing --plan")
		return protocol.ExitUsageError
	}

	taskID := strings.TrimSpace(positionalID)
	if taskID == "" {
		fmt.Fprintln(stderr, "missing task id")
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

	result, err := contextpkg.Explain(compiled, taskID)
	if err != nil {
		if errors.Is(err, contextpkg.ErrTaskNotFound) {
			fmt.Fprintln(stderr, err.Error())
			return protocol.ExitValidationFailed
		}
		fmt.Fprintf(stderr, "explain task: %v\n", err)
		return protocol.ExitInternalError
	}

	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		status := "ok"
		if !result.Runnable {
			status = "validation_failed"
		}
		payload := protocol.Envelope[contextpkg.ExplainResult]{
			SchemaVersion: protocol.SchemaVersionV01,
			ToolVersion:   CLIVersion,
			Command:       "explain",
			Status:        status,
			Data:          result,
		}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(payload); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return protocol.ExitInternalError
		}
	case "text":
		status := "runnable"
		if !result.Runnable {
			status = "blocked"
		}
		fmt.Fprintf(stdout, "task_id: %s\n", result.TaskID)
		fmt.Fprintf(stdout, "title: %s\n", result.Title)
		fmt.Fprintf(stdout, "horizon: %s\n", result.Horizon)
		fmt.Fprintf(stdout, "step_count: %d\n", result.StepCount)
		fmt.Fprintf(stdout, "evidence_refs: %d\n", len(result.EvidenceNodeRefs))
		fmt.Fprintf(stdout, "status: %s\n", status)
		if len(result.Blockers) > 0 {
			fmt.Fprintln(stdout, "blockers:")
			for _, blocker := range result.Blockers {
				fmt.Fprintf(stdout, "- %s: %s\n", blocker.Code, blocker.Message)
				if blocker.Suggestion != "" {
					fmt.Fprintf(stdout, "  suggestion: %s\n", blocker.Suggestion)
				}
			}
		}
	case "rich":
		status := "runnable"
		if !result.Runnable {
			status = "blocked"
		}
		fmt.Fprintln(stdout, "explain:")
		fmt.Fprintf(stdout, "  task_id: %s\n", result.TaskID)
		fmt.Fprintf(stdout, "  title: %s\n", result.Title)
		fmt.Fprintf(stdout, "  horizon: %s\n", result.Horizon)
		fmt.Fprintf(stdout, "  step_count: %d\n", result.StepCount)
		fmt.Fprintf(stdout, "  evidence_refs: %d\n", len(result.EvidenceNodeRefs))
		fmt.Fprintf(stdout, "  status: %s\n", status)
		fmt.Fprintf(stdout, "  blockers: %d\n", len(result.Blockers))
		if len(result.Blockers) > 0 {
			fmt.Fprintln(stdout, "  blocker_details:")
			for idx, blocker := range result.Blockers {
				fmt.Fprintf(stdout, "    %d. code: %s\n", idx+1, blocker.Code)
				fmt.Fprintf(stdout, "       message: %s\n", blocker.Message)
				if blocker.Suggestion != "" {
					fmt.Fprintf(stdout, "       suggestion: %s\n", blocker.Suggestion)
				}
			}
		}
	default:
		fmt.Fprintf(stderr, "invalid --format value: %s\n", *format)
		return protocol.ExitUsageError
	}

	if !result.Runnable {
		return protocol.ExitValidationFailed
	}
	return protocol.ExitSuccess
}
