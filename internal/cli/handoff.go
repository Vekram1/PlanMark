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

type handoffPacket struct {
	Query                string                       `json:"query"`
	PlanPath             string                       `json:"plan_path"`
	GlobalContextRefs    []string                     `json:"global_context_refs"`
	Need                 string                       `json:"need"`
	SelectedContextClass string                       `json:"selected_context_class"`
	SufficientForNeed    bool                         `json:"sufficient_for_need"`
	FallbackUsed         bool                         `json:"fallback_used"`
	FullPlanRequired     bool                         `json:"full_plan_required"`
	EscalationReasons    []string                     `json:"escalation_reasons,omitempty"`
	IncludedFiles        []string                     `json:"included_files,omitempty"`
	IncludedFileRefs     []contextpkg.IncludedFile    `json:"included_file_refs,omitempty"`
	IncludedDeps         []string                     `json:"included_deps,omitempty"`
	IncludedDepRefs      []contextpkg.IncludedDep     `json:"included_dep_refs,omitempty"`
	RemainingRisks       []string                     `json:"remaining_risks,omitempty"`
	NextUpgrade          string                       `json:"next_upgrade,omitempty"`
	Pins                 []contextpkg.PinExtract      `json:"pins,omitempty"`
	Closure              []contextpkg.L2Dependency    `json:"closure,omitempty"`
	Stats                contextpkg.ContextStats      `json:"stats"`
	Slice                contextpkg.OpenResult        `json:"slice"`
	Steps                []contextpkg.TaskStepContext `json:"steps,omitempty"`
	Evidence             []contextpkg.EvidenceSlice   `json:"evidence,omitempty"`
	DepsPointers         []string                     `json:"deps_pointers,omitempty"`
	Blockers             []contextpkg.ExplainBlocker  `json:"blockers,omitempty"`
	SuggestedMetadata    []string                     `json:"suggested_metadata,omitempty"`
}

func runHandoff(args []string, stdout io.Writer, stderr io.Writer) int {
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

	fs := flag.NewFlagSet("handoff", flag.ContinueOnError)
	fs.SetOutput(stderr)
	planPath := fs.String("plan", "", "path to `plan-file` markdown file")
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
		fmt.Fprintln(stderr, "usage: plan handoff <id|node-ref> --plan <path> [--format text|json]")
		return protocol.ExitUsageError
	}
	if len(remaining) == 1 {
		if positionalID != "" {
			fmt.Fprintln(stderr, "too many positional arguments for handoff")
			return protocol.ExitUsageError
		}
		positionalID = remaining[0]
	}
	if strings.TrimSpace(positionalID) == "" {
		fmt.Fprintln(stderr, "usage: plan handoff <id|node-ref> --plan <path> [--format text|json]")
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

	needPacket, err := contextpkg.SelectByNeed(compiled, positionalID, contextpkg.NeedHandoff)
	if err != nil {
		if errors.Is(err, contextpkg.ErrTaskNotReady) || errors.Is(err, contextpkg.ErrTaskNotFound) {
			fmt.Fprintln(stderr, err.Error())
			return protocol.ExitValidationFailed
		}
		fmt.Fprintf(stderr, "select handoff context: %v\n", err)
		return protocol.ExitInternalError
	}

	openResult, err := contextpkg.Open(compiled, positionalID)
	if err != nil {
		if errors.Is(err, contextpkg.ErrTaskNotFound) {
			fmt.Fprintln(stderr, err.Error())
			return protocol.ExitValidationFailed
		}
		fmt.Fprintf(stderr, "open source slice: %v\n", err)
		return protocol.ExitInternalError
	}

	packet := handoffPacket{
		Query:                strings.TrimSpace(positionalID),
		PlanPath:             compiled.PlanPath,
		GlobalContextRefs:    []string{"AGENTS.md", "docs/context/global.md"},
		Need:                 needPacket.Need,
		SelectedContextClass: needPacket.SelectedContextClass,
		SufficientForNeed:    needPacket.SufficientForNeed,
		FallbackUsed:         needPacket.FallbackUsed,
		FullPlanRequired:     needPacket.FullPlanRequired,
		EscalationReasons:    append([]string(nil), needPacket.EscalationReasons...),
		IncludedFiles:        append([]string(nil), needPacket.IncludedFiles...),
		IncludedFileRefs:     append([]contextpkg.IncludedFile(nil), needPacket.IncludedFileRefs...),
		IncludedDeps:         append([]string(nil), needPacket.IncludedDeps...),
		IncludedDepRefs:      append([]contextpkg.IncludedDep(nil), needPacket.IncludedDepRefs...),
		RemainingRisks:       append([]string(nil), needPacket.RemainingRisks...),
		NextUpgrade:          needPacket.NextUpgrade,
		Pins:                 append([]contextpkg.PinExtract(nil), needPacket.Pins...),
		Closure:              append([]contextpkg.L2Dependency(nil), needPacket.Closure...),
		Stats:                needPacket.Stats,
		Slice:                openResult,
		Steps:                append([]contextpkg.TaskStepContext(nil), openResult.Steps...),
		Evidence:             append([]contextpkg.EvidenceSlice(nil), openResult.Evidence...),
	}

	if strings.TrimSpace(openResult.TaskID) != "" {
		explainResult, err := contextpkg.Explain(compiled, openResult.TaskID)
		if err != nil {
			fmt.Fprintf(stderr, "explain task: %v\n", err)
			return protocol.ExitInternalError
		}
		packet.Blockers = append(packet.Blockers, explainResult.Blockers...)
		packet.SuggestedMetadata = append(packet.SuggestedMetadata, explainResult.SuggestedMetadata...)

		for _, task := range compiled.Semantic.Tasks {
			if strings.TrimSpace(task.ID) != strings.TrimSpace(openResult.TaskID) {
				continue
			}
			packet.DepsPointers = append(packet.DepsPointers, task.Deps...)
			break
		}
		sort.Strings(packet.DepsPointers)
	}

	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		payload := protocol.Envelope[handoffPacket]{
			SchemaVersion: protocol.SchemaVersionV01,
			ToolVersion:   CLIVersion,
			Command:       "handoff",
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
		fmt.Fprintf(stdout, "query: %s\n", packet.Query)
		fmt.Fprintf(stdout, "plan_path: %s\n", packet.PlanPath)
		fmt.Fprintf(stdout, "global_context_refs: %s\n", strings.Join(packet.GlobalContextRefs, ", "))
		fmt.Fprintf(stdout, "need: %s\n", packet.Need)
		fmt.Fprintf(stdout, "selected_context_class: %s\n", packet.SelectedContextClass)
		fmt.Fprintf(stdout, "sufficient_for_need: %t\n", packet.SufficientForNeed)
		fmt.Fprintf(stdout, "fallback_used: %t\n", packet.FallbackUsed)
		fmt.Fprintf(stdout, "full_plan_required: %t\n", packet.FullPlanRequired)
		fmt.Fprintf(stdout, "task_id: %s\n", packet.Slice.TaskID)
		fmt.Fprintf(stdout, "node_ref: %s\n", packet.Slice.NodeRef)
		fmt.Fprintf(stdout, "source_range: %d-%d\n", packet.Slice.StartLine, packet.Slice.EndLine)
		fmt.Fprintf(stdout, "slice_hash: %s\n", packet.Slice.SliceHash)
		fmt.Fprintf(stdout, "steps: %d\n", len(packet.Steps))
		fmt.Fprintf(stdout, "evidence: %d\n", len(packet.Evidence))
		fmt.Fprintf(stdout, "pins: %d\n", len(packet.Pins))
		fmt.Fprintf(stdout, "closure: %d\n", len(packet.Closure))
		fmt.Fprintf(stdout, "included_file_refs: %d\n", len(packet.IncludedFileRefs))
		fmt.Fprintf(stdout, "included_dep_refs: %d\n", len(packet.IncludedDepRefs))
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
		fmt.Fprintf(stdout, "deps_pointers: %d\n", len(packet.DepsPointers))
		fmt.Fprintf(stdout, "blockers: %d\n", len(packet.Blockers))
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
