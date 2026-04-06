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
	Query             string                       `json:"query"`
	PlanPath          string                       `json:"plan_path"`
	GlobalContextRefs []string                     `json:"global_context_refs"`
	Slice             contextpkg.OpenResult        `json:"slice"`
	Steps             []contextpkg.TaskStepContext `json:"steps,omitempty"`
	Evidence          []contextpkg.EvidenceSlice   `json:"evidence,omitempty"`
	DepsPointers      []string                     `json:"deps_pointers,omitempty"`
	Blockers          []contextpkg.ExplainBlocker  `json:"blockers,omitempty"`
	SuggestedMetadata []string                     `json:"suggested_metadata,omitempty"`
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
		Query:             strings.TrimSpace(positionalID),
		PlanPath:          compiled.PlanPath,
		GlobalContextRefs: []string{"AGENTS.md", "docs/context/global.md"},
		Slice:             openResult,
		Steps:             append([]contextpkg.TaskStepContext(nil), openResult.Steps...),
		Evidence:          append([]contextpkg.EvidenceSlice(nil), openResult.Evidence...),
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
		fmt.Fprintf(stdout, "task_id: %s\n", packet.Slice.TaskID)
		fmt.Fprintf(stdout, "node_ref: %s\n", packet.Slice.NodeRef)
		fmt.Fprintf(stdout, "source_range: %d-%d\n", packet.Slice.StartLine, packet.Slice.EndLine)
		fmt.Fprintf(stdout, "slice_hash: %s\n", packet.Slice.SliceHash)
		fmt.Fprintf(stdout, "steps: %d\n", len(packet.Steps))
		fmt.Fprintf(stdout, "evidence: %d\n", len(packet.Evidence))
		fmt.Fprintf(stdout, "deps_pointers: %d\n", len(packet.DepsPointers))
		fmt.Fprintf(stdout, "blockers: %d\n", len(packet.Blockers))
	default:
		fmt.Fprintf(stderr, "invalid --format value: %s\n", *format)
		return protocol.ExitUsageError
	}

	return protocol.ExitSuccess
}
