package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/compile"
	"github.com/vikramoddiraju/planmark/internal/protocol"
	"github.com/vikramoddiraju/planmark/internal/tracker"
)

type syncBeadsResult struct {
	Target       string `json:"target"`
	PlanPath     string `json:"plan_path"`
	StateDir     string `json:"state_dir,omitempty"`
	DryRun       bool   `json:"dry_run"`
	TasksSeen    int    `json:"tasks_seen"`
	TasksMutated int    `json:"tasks_mutated"`
	DriftCount   int    `json:"drift_count"`
	ManifestPath string `json:"manifest_path,omitempty"`
}

func runSync(args []string, stdout io.Writer, stderr io.Writer) int {
	target := ""
	filteredArgs := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--plan=") || strings.HasPrefix(arg, "--state-dir=") || strings.HasPrefix(arg, "--format=") {
			filteredArgs = append(filteredArgs, arg)
			continue
		}
		if arg == "--plan" || arg == "--state-dir" || arg == "--format" {
			filteredArgs = append(filteredArgs, arg)
			if i+1 < len(args) {
				i++
				filteredArgs = append(filteredArgs, args[i])
			}
			continue
		}
		if arg == "--dry-run" {
			filteredArgs = append(filteredArgs, arg)
			continue
		}
		if strings.HasPrefix(arg, "-") {
			filteredArgs = append(filteredArgs, arg)
			continue
		}
		if target == "" {
			target = arg
			continue
		}
		filteredArgs = append(filteredArgs, arg)
	}

	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(stderr)
	planPath := fs.String("plan", "", "path to PLAN markdown file")
	stateDir := fs.String("state-dir", ".planmark", "path to planmark local state directory")
	dryRun := fs.Bool("dry-run", false, "preview without writing sync manifest")
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
		fmt.Fprintln(stderr, "usage: plan sync beads --plan <path> [--state-dir <path>] [--dry-run] [--format text|json]")
		return protocol.ExitUsageError
	}
	if len(remaining) == 1 {
		if target != "" {
			fmt.Fprintln(stderr, "too many positional arguments for sync")
			return protocol.ExitUsageError
		}
		target = remaining[0]
	}
	if strings.TrimSpace(target) != "beads" {
		fmt.Fprintln(stderr, "usage: plan sync beads --plan <path> [--state-dir <path>] [--dry-run] [--format text|json]")
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

	nodesByRef := make(map[string]struct {
		startLine int
		endLine   int
		sliceHash string
	}, len(compiled.Source.Nodes))
	for _, node := range compiled.Source.Nodes {
		nodesByRef[node.NodeRef] = struct {
			startLine int
			endLine   int
			sliceHash string
		}{
			startLine: node.StartLine,
			endLine:   node.EndLine,
			sliceHash: node.SliceHash,
		}
	}

	adapter := tracker.NewBeadsAdapter()
	resolvedStateDir := strings.TrimSpace(*stateDir)
	if resolvedStateDir == "" {
		resolvedStateDir = ".planmark"
	}
	result := syncBeadsResult{
		Target:   "beads",
		PlanPath: *planPath,
		StateDir: resolvedStateDir,
		DryRun:   *dryRun,
	}
	ctx := context.Background()
	for _, task := range compiled.Semantic.Tasks {
		node, ok := nodesByRef[task.NodeRef]
		if !ok {
			fmt.Fprintf(stderr, "missing source node for task id %q (node_ref=%s)\n", task.ID, task.NodeRef)
			return protocol.ExitInternalError
		}
		pushResult, err := adapter.PushTask(ctx, tracker.TaskProjection{
			ID:              task.ID,
			Title:           task.Title,
			SourcePath:      compiled.PlanPath,
			SourceStartLine: node.startLine,
			SourceEndLine:   node.endLine,
			SourceHash:      node.sliceHash,
			Accept:          append([]string(nil), task.Accept...),
		})
		if err != nil {
			fmt.Fprintf(stderr, "push task projection: %v\n", err)
			return protocol.ExitInternalError
		}
		result.TasksSeen++
		if pushResult.Mutated {
			result.TasksMutated++
		}
		if strings.Contains(pushResult.Diagnostic, "drift detected") {
			result.DriftCount++
		}
	}

	if !*dryRun {
		manifestPath, err := adapter.WriteSyncManifest(resolvedStateDir)
		if err != nil {
			fmt.Fprintf(stderr, "write sync manifest: %v\n", err)
			return protocol.ExitInternalError
		}
		result.ManifestPath = manifestPath
	}

	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "text":
		fmt.Fprintf(stdout, "target: %s\n", result.Target)
		fmt.Fprintf(stdout, "plan_path: %s\n", result.PlanPath)
		fmt.Fprintf(stdout, "dry_run: %t\n", result.DryRun)
		fmt.Fprintf(stdout, "tasks_seen: %d\n", result.TasksSeen)
		fmt.Fprintf(stdout, "tasks_mutated: %d\n", result.TasksMutated)
		fmt.Fprintf(stdout, "drift_count: %d\n", result.DriftCount)
		if result.ManifestPath != "" {
			fmt.Fprintf(stdout, "manifest_path: %s\n", result.ManifestPath)
		}
	case "json":
		payload := protocol.Envelope[syncBeadsResult]{
			SchemaVersion: protocol.SchemaVersionV01,
			ToolVersion:   CLIVersion,
			Command:       "sync beads",
			Status:        "ok",
			Data:          result,
		}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(payload); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return protocol.ExitInternalError
		}
	default:
		fmt.Fprintf(stderr, "invalid --format value: %s\n", *format)
		return protocol.ExitUsageError
	}

	return protocol.ExitSuccess
}
