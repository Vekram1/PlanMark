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
	"github.com/vikramoddiraju/planmark/internal/doctor"
	"github.com/vikramoddiraju/planmark/internal/protocol"
)

type queryResult struct {
	PlanPath string             `json:"plan_path"`
	Filters  queryFilters       `json:"filters"`
	Count    int                `json:"count"`
	Tasks    []doctor.QueryTask `json:"tasks"`
}

type queryFilters struct {
	Horizon string `json:"horizon,omitempty"`
	Ready   bool   `json:"ready"`
	Blocked bool   `json:"blocked"`
}

func runQuery(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("query", flag.ContinueOnError)
	fs.SetOutput(stderr)
	planPath := fs.String("plan", "", "path to `plan-file` markdown file")
	horizon := fs.String("horizon", "", "horizon filter: now|next|later")
	ready := fs.Bool("ready", false, "include only ready tasks")
	blocked := fs.Bool("blocked", false, "include only blocked tasks")
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
	if *ready && *blocked {
		fmt.Fprintln(stderr, "invalid filter combination: --ready and --blocked are mutually exclusive")
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

	tasks, err := doctor.QueryTasks(compiled, *horizon)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitUsageError
	}

	filtered := make([]doctor.QueryTask, 0, len(tasks))
	for _, task := range tasks {
		if *ready && !task.Ready {
			continue
		}
		if *blocked && !task.Blocked {
			continue
		}
		filtered = append(filtered, task)
	}

	result := queryResult{
		PlanPath: compiled.PlanPath,
		Filters: queryFilters{
			Horizon: strings.ToLower(strings.TrimSpace(*horizon)),
			Ready:   *ready,
			Blocked: *blocked,
		},
		Count: len(filtered),
		Tasks: filtered,
	}

	switch *format {
	case "json":
		payload := protocol.Envelope[queryResult]{
			SchemaVersion: protocol.SchemaVersionV01,
			ToolVersion:   CLIVersion,
			Command:       "query",
			Status:        "ok",
			Data:          result,
		}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(payload); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return protocol.ExitInternalError
		}
		return protocol.ExitSuccess
	case "text":
		fmt.Fprintf(stdout, "plan_path: %s\n", result.PlanPath)
		if result.Filters.Horizon != "" {
			fmt.Fprintf(stdout, "horizon: %s\n", result.Filters.Horizon)
		}
		fmt.Fprintf(stdout, "ready_filter: %t\n", result.Filters.Ready)
		fmt.Fprintf(stdout, "blocked_filter: %t\n", result.Filters.Blocked)
		fmt.Fprintf(stdout, "count: %d\n", result.Count)
		for _, task := range result.Tasks {
			horizonLabel := task.Horizon
			if horizonLabel == "" {
				horizonLabel = "-"
			}
			fmt.Fprintf(stdout, "- %s [%s] ready=%t blocked=%t", task.ID, horizonLabel, task.Ready, task.Blocked)
			if task.Title != "" {
				fmt.Fprintf(stdout, " title=%q", task.Title)
			}
			if len(task.Deps) > 0 {
				fmt.Fprintf(stdout, " deps=%s", strings.Join(task.Deps, ","))
			}
			fmt.Fprintln(stdout)
		}
		return protocol.ExitSuccess
	default:
		fmt.Fprintf(stderr, "invalid --format value: %s\n", *format)
		return protocol.ExitUsageError
	}
}
