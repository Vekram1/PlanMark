package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/compile"
	contextpkg "github.com/vikramoddiraju/planmark/internal/context"
	"github.com/vikramoddiraju/planmark/internal/protocol"
)

func runContext(args []string, stdout io.Writer, stderr io.Writer) int {
	positionalID := ""
	filteredArgs := make([]string, 0, len(args))
	needExplicit := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if longFlagHasAttachedValueAny(arg, "plan", "need", "format") {
			if longFlagHasAttachedValue(arg, "need") {
				needExplicit = true
			}
			filteredArgs = append(filteredArgs, arg)
			continue
		}
		if longFlagMatches(arg, "plan", "need", "format") {
			if longFlagMatches(arg, "need") {
				needExplicit = true
			}
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

	fs := flag.NewFlagSet("context", flag.ContinueOnError)
	setSilentFlagOutput(fs)
	planPath := fs.String("plan", "", "path to the plan markdown file")
	need := fs.String("need", "", "context need: execute|edit|dependency-check|handoff|auto (defaults to auto when omitted)")
	format := fs.String("format", "text", "output format: text|json")
	if err := fs.Parse(filteredArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(stdout)
			fs.PrintDefaults()
			return protocol.ExitSuccess
		}
		printFlagParseError(stderr, err)
		return protocol.ExitUsageError
	}

	remaining := fs.Args()
	if len(remaining) > 1 {
		fmt.Fprintln(stderr, "usage: plan context <id> --plan <path> [--need execute|edit|dependency-check|handoff|auto] [--format text|json]")
		return protocol.ExitUsageError
	}
	if len(remaining) == 1 {
		if positionalID != "" {
			fmt.Fprintln(stderr, "too many positional arguments for context")
			return protocol.ExitUsageError
		}
		positionalID = remaining[0]
	}
	if strings.TrimSpace(positionalID) == "" {
		fmt.Fprintln(stderr, "usage: plan context <id> --plan <path> [--need execute|edit|dependency-check|handoff|auto] [--format text|json]")
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

	needNormalized := strings.TrimSpace(*need)
	if !needExplicit {
		needNormalized = string(contextpkg.NeedAuto)
	}
	parsedNeed, err := contextpkg.ParseNeed(needNormalized)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitUsageError
	}

	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		packet, err := contextpkg.SelectByNeed(compiled, taskID, parsedNeed)
		if err != nil {
			if errors.Is(err, contextpkg.ErrTaskNotReady) || errors.Is(err, contextpkg.ErrTaskNotFound) {
				fmt.Fprintln(stderr, err.Error())
				return protocol.ExitValidationFailed
			}
			fmt.Fprintf(stderr, "build context: %v\n", err)
			return protocol.ExitInternalError
		}
		payload := protocol.Envelope[contextpkg.NeedPacket]{
			SchemaVersion: protocol.SchemaVersionV01,
			ToolVersion:   CLIVersion,
			Command:       "context",
			Status:        "ok",
			Data:          packet,
		}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(payload); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return protocol.ExitInternalError
		}
	case "text":
		packet, err := contextpkg.SelectByNeed(compiled, taskID, parsedNeed)
		if err != nil {
			if errors.Is(err, contextpkg.ErrTaskNotReady) || errors.Is(err, contextpkg.ErrTaskNotFound) {
				fmt.Fprintln(stderr, err.Error())
				return protocol.ExitValidationFailed
			}
			fmt.Fprintf(stderr, "build context: %v\n", err)
			return protocol.ExitInternalError
		}
		fmt.Fprintf(stdout, "need: %s\n", packet.Need)
		fmt.Fprintf(stdout, "selected_context_class: %s\n", packet.SelectedContextClass)
		fmt.Fprintf(stdout, "sufficient_for_need: %t\n", packet.SufficientForNeed)
		fmt.Fprintf(stdout, "fallback_used: %t\n", packet.FallbackUsed)
		fmt.Fprintf(stdout, "full_plan_required: %t\n", packet.FullPlanRequired)
		fmt.Fprintf(stdout, "query: %s\n", packet.Query)
		fmt.Fprintf(stdout, "task_id: %s\n", packet.TaskID)
		fmt.Fprintf(stdout, "title: %s\n", packet.Title)
		fmt.Fprintf(stdout, "horizon: %s\n", packet.Horizon)
		fmt.Fprintf(stdout, "sections: %d\n", len(packet.Sections))
		fmt.Fprintf(stdout, "steps: %d\n", len(packet.Steps))
		fmt.Fprintf(stdout, "evidence: %d\n", len(packet.Evidence))
		fmt.Fprintf(stdout, "source_path: %s\n", packet.SourcePath)
		fmt.Fprintf(stdout, "source_range: %d-%d\n", packet.StartLine, packet.EndLine)
		fmt.Fprintf(stdout, "slice_hash: %s\n", packet.SliceHash)
		fmt.Fprintf(stdout, "pins: %d\n", len(packet.Pins))
		fmt.Fprintf(stdout, "dependencies: %d\n", len(packet.Dependencies))
		fmt.Fprintf(stdout, "dependents: %d\n", len(packet.Dependents))
		fmt.Fprintf(stdout, "closure: %d\n", len(packet.Closure))
		fmt.Fprintf(stdout, "included_file_refs: %d\n", len(packet.IncludedFileRefs))
		fmt.Fprintf(stdout, "included_dep_refs: %d\n", len(packet.IncludedDepRefs))
		fmt.Fprintf(stdout, "included_dependent_refs: %d\n", len(packet.IncludedDependentRefs))
		fmt.Fprintf(stdout, "stats.included_lines: %d\n", packet.Stats.IncludedLines)
		fmt.Fprintf(stdout, "stats.included_files_count: %d\n", packet.Stats.IncludedFilesCount)
		fmt.Fprintf(stdout, "stats.included_deps_count: %d\n", packet.Stats.IncludedDepsCount)
		fmt.Fprintf(stdout, "stats.estimated_token_count: %d\n", packet.Stats.EstimatedTokenCount)
		if len(packet.Stats.EscalationPath) > 0 {
			fmt.Fprintf(stdout, "stats.escalation_path: %s\n", strings.Join(packet.Stats.EscalationPath, " -> "))
		}
		if packet.Stats.FullPlanLines > 0 {
			fmt.Fprintf(stdout, "stats.full_plan_lines: %d\n", packet.Stats.FullPlanLines)
			fmt.Fprintf(stdout, "stats.saved_lines_vs_full_plan: %d\n", packet.Stats.SavedLinesVsFullPlan)
		}
		if packet.Stats.FullPlanEstimatedTokens > 0 {
			fmt.Fprintf(stdout, "stats.full_plan_estimated_tokens: %d\n", packet.Stats.FullPlanEstimatedTokens)
			fmt.Fprintf(stdout, "stats.saved_tokens_vs_full_plan: %d\n", packet.Stats.SavedTokensVsFullPlan)
		}
		if len(packet.EscalationReasons) > 0 {
			fmt.Fprintf(stdout, "escalation_reasons: %s\n", strings.Join(packet.EscalationReasons, ", "))
		}
		if len(packet.RemainingRisks) > 0 {
			fmt.Fprintf(stdout, "remaining_risks: %s\n", strings.Join(packet.RemainingRisks, ", "))
		}
		if packet.NextUpgrade != "" {
			fmt.Fprintf(stdout, "next_upgrade: %s\n", packet.NextUpgrade)
		}
	default:
		fmt.Fprintf(stderr, "invalid --format value: %s\n", *format)
		return protocol.ExitUsageError
	}
	return protocol.ExitSuccess
}

func longFlagHasAttachedValueAny(arg string, names ...string) bool {
	for _, name := range names {
		if longFlagHasAttachedValue(arg, name) {
			return true
		}
	}
	return false
}

func longFlagHasAttachedValue(arg string, name string) bool {
	return strings.HasPrefix(arg, "--"+name+"=")
}

func longFlagMatches(arg string, names ...string) bool {
	for _, name := range names {
		if arg == "--"+name {
			return true
		}
	}
	return false
}

func setSilentFlagOutput(fs *flag.FlagSet) {
	fs.SetOutput(ioutil.Discard)
}

func printFlagParseError(stderr io.Writer, err error) {
	fmt.Fprintln(stderr, err.Error())
}
