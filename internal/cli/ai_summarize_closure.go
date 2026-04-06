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
	"github.com/vikramoddiraju/planmark/internal/ir"
	"github.com/vikramoddiraju/planmark/internal/protocol"
)

type closureSummaryEntry struct {
	TaskID     string `json:"task_id"`
	Title      string `json:"title,omitempty"`
	Horizon    string `json:"horizon,omitempty"`
	NodeRef    string `json:"node_ref"`
	SourcePath string `json:"source_path"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	SliceHash  string `json:"slice_hash"`
	Summary    string `json:"summary"`
}

type summarizeClosureResult struct {
	TaskID       string                `json:"task_id"`
	PlanPath     string                `json:"plan_path"`
	Title        string                `json:"title,omitempty"`
	Horizon      string                `json:"horizon,omitempty"`
	ClosureCount int                   `json:"closure_count"`
	Closure      []closureSummaryEntry `json:"closure,omitempty"`
}

func runAISummarizeClosure(args []string, stdout io.Writer, stderr io.Writer) int {
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

	fs := flag.NewFlagSet("ai summarize-closure", flag.ContinueOnError)
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
		fmt.Fprintln(stderr, "usage: plan ai summarize-closure <id> --plan <path> [--format text|json]")
		return protocol.ExitUsageError
	}
	if len(remaining) == 1 {
		if positionalID != "" {
			fmt.Fprintln(stderr, "too many positional arguments for ai summarize-closure")
			return protocol.ExitUsageError
		}
		positionalID = remaining[0]
	}
	if strings.TrimSpace(positionalID) == "" {
		fmt.Fprintln(stderr, "usage: plan ai summarize-closure <id> --plan <path> [--format text|json]")
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
	taskByID := make(map[string]ir.Task, len(compiled.Semantic.Tasks))
	for _, task := range compiled.Semantic.Tasks {
		taskByID[strings.TrimSpace(task.ID)] = task
	}
	rootTask, ok := taskByID[taskID]
	if !ok {
		fmt.Fprintf(stderr, "task not found: %s\n", taskID)
		return protocol.ExitValidationFailed
	}

	nodeByRef := make(map[string]ir.SourceNode, len(compiled.Source.Nodes))
	for _, sourceNode := range compiled.Source.Nodes {
		nodeByRef[sourceNode.NodeRef] = sourceNode
	}

	visited := make(map[string]struct{})
	var visit func(string) error
	visit = func(id string) error {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil
		}
		if _, seen := visited[id]; seen {
			return nil
		}
		depTask, ok := taskByID[id]
		if !ok {
			return fmt.Errorf("dependency task not found: %s", id)
		}
		visited[id] = struct{}{}
		next := append([]string(nil), depTask.Deps...)
		sort.Strings(next)
		for _, depID := range next {
			if err := visit(depID); err != nil {
				return err
			}
		}
		return nil
	}

	rootDeps := append([]string(nil), rootTask.Deps...)
	sort.Strings(rootDeps)
	for _, depID := range rootDeps {
		if err := visit(depID); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return protocol.ExitValidationFailed
		}
	}

	orderedIDs := make([]string, 0, len(visited))
	for id := range visited {
		orderedIDs = append(orderedIDs, id)
	}
	sort.Strings(orderedIDs)

	closure := make([]closureSummaryEntry, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		dep := taskByID[id]
		depNode, ok := nodeByRef[dep.NodeRef]
		if !ok {
			fmt.Fprintf(stderr, "source node missing for dependency task %q (node_ref=%s)\n", dep.ID, dep.NodeRef)
			return protocol.ExitValidationFailed
		}
		summary := dep.ID
		if dep.Title != "" {
			summary = dep.ID + ": " + dep.Title
		}
		if dep.Horizon != "" {
			summary += " [" + dep.Horizon + "]"
		}
		closure = append(closure, closureSummaryEntry{
			TaskID:     dep.ID,
			Title:      dep.Title,
			Horizon:    dep.Horizon,
			NodeRef:    dep.NodeRef,
			SourcePath: compiled.PlanPath,
			StartLine:  depNode.StartLine,
			EndLine:    depNode.EndLine,
			SliceHash:  depNode.SliceHash,
			Summary:    summary,
		})
	}

	result := summarizeClosureResult{
		TaskID:       rootTask.ID,
		PlanPath:     compiled.PlanPath,
		Title:        rootTask.Title,
		Horizon:      rootTask.Horizon,
		ClosureCount: len(closure),
		Closure:      closure,
	}

	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		payload := protocol.Envelope[summarizeClosureResult]{
			SchemaVersion: protocol.SchemaVersionV01,
			ToolVersion:   CLIVersion,
			Command:       "ai summarize-closure",
			Status:        "ok",
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
		fmt.Fprintf(stdout, "title: %s\n", result.Title)
		fmt.Fprintf(stdout, "horizon: %s\n", result.Horizon)
		fmt.Fprintf(stdout, "closure_count: %d\n", result.ClosureCount)
		if len(result.Closure) > 0 {
			fmt.Fprintln(stdout, "closure:")
			for _, dep := range result.Closure {
				fmt.Fprintf(stdout, "- %s\n", dep.Summary)
				fmt.Fprintf(stdout, "  source: %s:%d-%d\n", dep.SourcePath, dep.StartLine, dep.EndLine)
				fmt.Fprintf(stdout, "  node_ref: %s\n", dep.NodeRef)
			}
		}
	default:
		fmt.Fprintf(stderr, "invalid --format value: %s\n", *format)
		return protocol.ExitUsageError
	}

	return protocol.ExitSuccess
}
