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

type draftBeadSuggestion struct {
	TaskID            string `json:"task_id"`
	Title             string `json:"title"`
	Horizon           string `json:"horizon,omitempty"`
	SuggestedTitle    string `json:"suggested_title"`
	SuggestedType     string `json:"suggested_type"`
	SuggestedPriority int    `json:"suggested_priority"`
	SuggestedBody     string `json:"suggested_body"`
}

type draftBeadsResult struct {
	PlanPath     string                `json:"plan_path"`
	Horizon      string                `json:"horizon_filter"`
	Limit        int                   `json:"limit"`
	Suggestions  []draftBeadSuggestion `json:"suggestions,omitempty"`
	TotalScanned int                   `json:"total_scanned"`
}

func runAIDraftBeads(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("ai draft-beads", flag.ContinueOnError)
	fs.SetOutput(stderr)
	planPath := fs.String("plan", "", "path to PLAN markdown file")
	format := fs.String("format", "text", "output format: text|json")
	horizon := fs.String("horizon", "all", "horizon filter: all|now|next|later")
	limit := fs.Int("limit", 20, "max suggestions to emit")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return protocol.ExitSuccess
		}
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitUsageError
	}

	if strings.TrimSpace(*planPath) == "" {
		fmt.Fprintln(stderr, "missing --plan")
		return protocol.ExitUsageError
	}
	if *limit <= 0 {
		fmt.Fprintln(stderr, "--limit must be > 0")
		return protocol.ExitUsageError
	}

	horizonFilter := strings.ToLower(strings.TrimSpace(*horizon))
	switch horizonFilter {
	case "all", "now", "next", "later":
	default:
		fmt.Fprintf(stderr, "invalid --horizon value: %s\n", *horizon)
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

	candidates := make([]ir.Task, 0, len(compiled.Semantic.Tasks))
	for _, task := range compiled.Semantic.Tasks {
		if horizonFilter != "all" && strings.ToLower(strings.TrimSpace(task.Horizon)) != horizonFilter {
			continue
		}
		candidates = append(candidates, task)
	}
	sort.Slice(candidates, func(i, j int) bool {
		leftPriority := horizonPriority(candidates[i].Horizon)
		rightPriority := horizonPriority(candidates[j].Horizon)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		left := canonicalTaskID(candidates[i])
		right := canonicalTaskID(candidates[j])
		if left == right {
			return strings.TrimSpace(candidates[i].Title) < strings.TrimSpace(candidates[j].Title)
		}
		return left < right
	})

	suggestions := make([]draftBeadSuggestion, 0, minInt(*limit, len(candidates)))
	for _, task := range candidates {
		if len(suggestions) >= *limit {
			break
		}
		suggestions = append(suggestions, buildDraftBeadSuggestion(task))
	}

	result := draftBeadsResult{
		PlanPath:     compiled.PlanPath,
		Horizon:      horizonFilter,
		Limit:        *limit,
		Suggestions:  suggestions,
		TotalScanned: len(candidates),
	}

	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		payload := protocol.Envelope[draftBeadsResult]{
			SchemaVersion: protocol.SchemaVersionV01,
			ToolVersion:   CLIVersion,
			Command:       "ai draft-beads",
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
		fmt.Fprintf(stdout, "plan_path: %s\n", result.PlanPath)
		fmt.Fprintf(stdout, "horizon_filter: %s\n", result.Horizon)
		fmt.Fprintf(stdout, "limit: %d\n", result.Limit)
		fmt.Fprintf(stdout, "total_scanned: %d\n", result.TotalScanned)
		fmt.Fprintf(stdout, "suggestion_count: %d\n", len(result.Suggestions))
		if len(result.Suggestions) > 0 {
			fmt.Fprintln(stdout, "suggestions:")
			for _, suggestion := range result.Suggestions {
				fmt.Fprintf(stdout, "- %s\n", suggestion.SuggestedTitle)
				fmt.Fprintf(stdout, "  type: %s\n", suggestion.SuggestedType)
				fmt.Fprintf(stdout, "  priority: %d\n", suggestion.SuggestedPriority)
				fmt.Fprintf(stdout, "  body: %s\n", suggestion.SuggestedBody)
			}
		}
	default:
		fmt.Fprintf(stderr, "invalid --format value: %s\n", *format)
		return protocol.ExitUsageError
	}

	return protocol.ExitSuccess
}

func buildDraftBeadSuggestion(task ir.Task) draftBeadSuggestion {
	taskID := canonicalTaskID(task)
	title := strings.TrimSpace(task.Title)
	if title == "" {
		title = taskID
	}

	priority := horizonPriority(task.Horizon)

	lines := []string{
		fmt.Sprintf("PLAN TRACE: %s", taskID),
		fmt.Sprintf("Source title: %s", title),
		fmt.Sprintf("Horizon: %s", strings.TrimSpace(task.Horizon)),
	}
	if len(task.Deps) > 0 {
		deps := append([]string(nil), task.Deps...)
		sort.Strings(deps)
		lines = append(lines, fmt.Sprintf("Deps: %s", strings.Join(deps, ", ")))
	}
	if len(task.Accept) > 0 {
		lines = append(lines, fmt.Sprintf("Acceptance lines: %d", len(task.Accept)))
	} else {
		lines = append(lines, "Acceptance lines: 0 (add explicit checks before execution)")
	}
	if len(task.Deps) >= 3 {
		lines = append(lines, "Split hint: consider splitting into sub-beads by dependency cluster.")
	}

	return draftBeadSuggestion{
		TaskID:            taskID,
		Title:             title,
		Horizon:           strings.TrimSpace(task.Horizon),
		SuggestedTitle:    fmt.Sprintf("[%s] %s", taskID, title),
		SuggestedType:     "task",
		SuggestedPriority: priority,
		SuggestedBody:     strings.Join(lines, "\n"),
	}
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func horizonPriority(horizon string) int {
	switch strings.ToLower(strings.TrimSpace(horizon)) {
	case "now":
		return 1
	case "next":
		return 2
	case "later":
		return 3
	default:
		return 4
	}
}

func canonicalTaskID(task ir.Task) string {
	id := strings.TrimSpace(task.ID)
	if id != "" {
		return id
	}
	nodeRef := strings.TrimSpace(task.NodeRef)
	if nodeRef != "" {
		return nodeRef
	}
	return "(no-id)"
}
