package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/compile"
	contextpkg "github.com/vikramoddiraju/planmark/internal/context"
	"github.com/vikramoddiraju/planmark/internal/protocol"
)

type suggestAcceptResult struct {
	TaskID      string   `json:"task_id"`
	PlanPath    string   `json:"plan_path"`
	Horizon     string   `json:"horizon"`
	Runnable    bool     `json:"runnable"`
	Suggestions []string `json:"suggestions"`
}

func runAI(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: plan ai <subcommand> [args]")
		fmt.Fprintln(stderr, "subcommands: suggest-accept, summarize-closure, draft-beads")
		return protocol.ExitUsageError
	}

	switch args[0] {
	case "suggest-accept":
		return runAISuggestAccept(args[1:], stdout, stderr)
	case "summarize-closure":
		return runAISummarizeClosure(args[1:], stdout, stderr)
	case "draft-beads":
		return runAIDraftBeads(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown ai command: %s\n", args[0])
		return protocol.ExitUsageError
	}
}

func runAISuggestAccept(args []string, stdout io.Writer, stderr io.Writer) int {
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

	fs := flag.NewFlagSet("ai suggest-accept", flag.ContinueOnError)
	fs.SetOutput(stderr)
	planPath := fs.String("plan", "", "path to PLAN markdown file")
	format := fs.String("format", "text", "output format: text|json")
	if err := fs.Parse(filteredArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return protocol.ExitSuccess
		}
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitUsageError
	}

	remaining := fs.Args()
	if len(remaining) > 1 {
		fmt.Fprintln(stderr, "usage: plan ai suggest-accept <id> --plan <path> [--format text|json]")
		return protocol.ExitUsageError
	}
	if len(remaining) == 1 {
		if positionalID != "" {
			fmt.Fprintln(stderr, "too many positional arguments for ai suggest-accept")
			return protocol.ExitUsageError
		}
		positionalID = remaining[0]
	}
	if strings.TrimSpace(positionalID) == "" {
		fmt.Fprintln(stderr, "usage: plan ai suggest-accept <id> --plan <path> [--format text|json]")
		return protocol.ExitUsageError
	}
	if strings.TrimSpace(*planPath) == "" {
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

	taskID := strings.TrimSpace(positionalID)
	explain, err := contextpkg.Explain(compiled, taskID)
	if err != nil {
		if errors.Is(err, contextpkg.ErrTaskNotFound) {
			fmt.Fprintln(stderr, err.Error())
			return protocol.ExitValidationFailed
		}
		fmt.Fprintf(stderr, "explain task: %v\n", err)
		return protocol.ExitInternalError
	}

	suggestions := suggestAcceptLines(explain.Blockers)
	if len(suggestions) == 0 {
		suggestions = []string{
			"@accept cmd:<command>",
		}
	}

	result := suggestAcceptResult{
		TaskID:      explain.TaskID,
		PlanPath:    compiled.PlanPath,
		Horizon:     explain.Horizon,
		Runnable:    explain.Runnable,
		Suggestions: suggestions,
	}

	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		status := "ok"
		if len(result.Suggestions) == 0 {
			status = "validation_failed"
		}
		payload := protocol.Envelope[suggestAcceptResult]{
			SchemaVersion: protocol.SchemaVersionV01,
			ToolVersion:   CLIVersion,
			Command:       "ai suggest-accept",
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
		fmt.Fprintf(stdout, "task_id: %s\n", result.TaskID)
		fmt.Fprintf(stdout, "plan_path: %s\n", result.PlanPath)
		fmt.Fprintf(stdout, "horizon: %s\n", result.Horizon)
		fmt.Fprintf(stdout, "runnable: %t\n", result.Runnable)
		fmt.Fprintln(stdout, "suggestions:")
		for _, suggestion := range result.Suggestions {
			fmt.Fprintf(stdout, "- %s\n", suggestion)
		}
	default:
		fmt.Fprintf(stderr, "invalid --format value: %s\n", *format)
		return protocol.ExitUsageError
	}

	return protocol.ExitSuccess
}

func suggestAcceptLines(blockers []contextpkg.ExplainBlocker) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 4)
	for _, blocker := range blockers {
		for _, suggestion := range blockerSuggestions(blocker.Code) {
			if _, ok := seen[suggestion]; ok {
				continue
			}
			seen[suggestion] = struct{}{}
			out = append(out, suggestion)
		}
	}
	sort.Strings(out)
	return out
}

func blockerSuggestions(code string) []string {
	switch strings.TrimSpace(code) {
	case "MISSING_ACCEPT":
		return []string{
			"@accept cmd:<command>",
			"@accept file:<path> exists",
		}
	case "MISSING_DEP":
		return []string{
			"@accept cmd:plan doctor --plan <path> --profile exec",
		}
	default:
		return nil
	}
}
