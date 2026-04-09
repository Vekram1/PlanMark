package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/compile"
	"github.com/vikramoddiraju/planmark/internal/protocol"
	"github.com/vikramoddiraju/planmark/internal/tracker"
)

type cleanupResult struct {
	Target           string                     `json:"target"`
	PlanPath         string                     `json:"plan_path"`
	DryRun           bool                       `json:"dry_run"`
	CandidatesSeen   int                        `json:"candidates_seen"`
	CandidatesClosed int                        `json:"candidates_closed"`
	Candidates       []tracker.CleanupCandidate `json:"candidates,omitempty"`
}

func runCleanup(args []string, stdout io.Writer, stderr io.Writer) int {
	target := ""
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

	fs := flag.NewFlagSet("cleanup", flag.ContinueOnError)
	fs.SetOutput(stderr)
	planPath := fs.String("plan", "", "path to `plan-file` markdown file")
	dryRun := fs.Bool("dry-run", false, "preview cleanup candidates without closing them")
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
		fmt.Fprintln(stderr, "usage: plan cleanup beads --plan <path> [--dry-run] [--format text|json]")
		return protocol.ExitUsageError
	}
	if len(remaining) == 1 {
		if target != "" {
			fmt.Fprintln(stderr, "too many positional arguments for cleanup")
			return protocol.ExitUsageError
		}
		target = remaining[0]
	}
	if strings.TrimSpace(target) == "" {
		target = "beads"
	}
	if target != "beads" {
		fmt.Fprintln(stderr, "cleanup currently supports only the beads tracker")
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
	desiredIDs := make(map[string]struct{}, len(compiled.Semantic.Tasks))
	for _, task := range compiled.Semantic.Tasks {
		desiredIDs[task.ID] = struct{}{}
	}

	adapter, err := instantiateSyncAdapter("beads")
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitUsageError
	}
	if dbConfigurer, ok := adapter.(trackerDBPathConfigurer); ok {
		dbConfigurer.SetDBPath(filepath.Join(".beads", "beads.db"))
	}
	lister, ok := adapter.(cleanupCandidateLister)
	if !ok {
		fmt.Fprintln(stderr, "cleanup is not supported by this tracker adapter")
		return protocol.ExitUsageError
	}
	candidates, err := lister.ListCleanupCandidates(context.Background(), *planPath, desiredIDs)
	if err != nil {
		fmt.Fprintf(stderr, "list cleanup candidates: %v\n", err)
		return protocol.ExitInternalError
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Reason != candidates[j].Reason {
			return candidates[i].Reason < candidates[j].Reason
		}
		if candidates[i].ExternalRef != candidates[j].ExternalRef {
			return candidates[i].ExternalRef < candidates[j].ExternalRef
		}
		if candidates[i].SourcePath != candidates[j].SourcePath {
			return candidates[i].SourcePath < candidates[j].SourcePath
		}
		return candidates[i].RemoteID < candidates[j].RemoteID
	})

	result := cleanupResult{
		Target:           "beads",
		PlanPath:         *planPath,
		DryRun:           *dryRun,
		CandidatesSeen:   len(candidates),
		Candidates:       candidates,
		CandidatesClosed: 0,
	}

	if !*dryRun {
		closer, ok := adapter.(cleanupCandidateCloser)
		if !ok {
			fmt.Fprintln(stderr, "cleanup apply is not supported by this tracker adapter")
			return protocol.ExitUsageError
		}
		for _, candidate := range candidates {
			if err := closer.CleanupCandidate(context.Background(), candidate); err != nil {
				fmt.Fprintf(stderr, "cleanup candidate %s: %v\n", candidate.RemoteID, err)
				return protocol.ExitInternalError
			}
			result.CandidatesClosed++
		}
	}

	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		payload := protocol.Envelope[cleanupResult]{
			SchemaVersion: protocol.SchemaVersionV01,
			ToolVersion:   CLIVersion,
			Command:       "cleanup beads",
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
		fmt.Fprintf(stdout, "target: %s\n", result.Target)
		fmt.Fprintf(stdout, "plan_path: %s\n", result.PlanPath)
		fmt.Fprintf(stdout, "dry_run: %t\n", result.DryRun)
		fmt.Fprintf(stdout, "candidates_seen: %d\n", result.CandidatesSeen)
		fmt.Fprintf(stdout, "candidates_closed: %d\n", result.CandidatesClosed)
		for _, candidate := range result.Candidates {
			fmt.Fprintf(stdout, "- remote_id=%s reason=%s", candidate.RemoteID, candidate.Reason)
			if candidate.ExternalRef != "" {
				fmt.Fprintf(stdout, " external_ref=%s", candidate.ExternalRef)
			}
			if candidate.SourcePath != "" {
				fmt.Fprintf(stdout, " source_path=%s", candidate.SourcePath)
			}
			if candidate.Title != "" {
				fmt.Fprintf(stdout, " title=%q", candidate.Title)
			}
			fmt.Fprintln(stdout)
		}
	default:
		fmt.Fprintf(stderr, "invalid --format value: %s\n", *format)
		return protocol.ExitUsageError
	}

	return protocol.ExitSuccess
}
