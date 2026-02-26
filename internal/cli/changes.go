package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/build"
	"github.com/vikramoddiraju/planmark/internal/change"
	"github.com/vikramoddiraju/planmark/internal/compile"
	"github.com/vikramoddiraju/planmark/internal/fsio"
	"github.com/vikramoddiraju/planmark/internal/ir"
	"github.com/vikramoddiraju/planmark/internal/protocol"
)

type changesResult struct {
	PlanPath          string              `json:"plan_path"`
	StateDir          string              `json:"state_dir"`
	SinceCompileID    string              `json:"since_compile_id,omitempty"`
	SinceGitRef       string              `json:"since_git_ref,omitempty"`
	BaselineCompileID string              `json:"baseline_compile_id,omitempty"`
	CurrentCompileID  string              `json:"current_compile_id"`
	AdvisoryHunks     int                 `json:"advisory_hunks,omitempty"`
	Changes           []change.TaskChange `json:"changes"`
}

func runChanges(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("changes", flag.ContinueOnError)
	fs.SetOutput(stderr)
	planPath := fs.String("plan", "PLAN.md", "path to PLAN markdown file")
	stateDir := fs.String("state-dir", ".planmark", "path to planmark local state directory")
	since := fs.String("since", "", "baseline compile id or git ref for deterministic comparison")
	format := fs.String("format", "text", "output format: text|json")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return protocol.ExitSuccess
		}
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitUsageError
	}
	if len(fs.Args()) != 0 {
		fmt.Fprintln(stderr, "usage: plan changes [--plan <path>] [--state-dir <path>] [--since <compile-id|git-ref>] [--format text|json]")
		return protocol.ExitUsageError
	}

	resolvedPlanPath := strings.TrimSpace(*planPath)
	if resolvedPlanPath == "" {
		fmt.Fprintln(stderr, "missing --plan")
		return protocol.ExitUsageError
	}
	resolvedStateDir := strings.TrimSpace(*stateDir)
	if resolvedStateDir == "" {
		resolvedStateDir = ".planmark"
	}

	content, err := os.ReadFile(resolvedPlanPath)
	if err != nil {
		fmt.Fprintf(stderr, "read plan: %v\n", err)
		return protocol.ExitInternalError
	}
	currentPlan, err := compile.CompilePlan(resolvedPlanPath, content, compile.NewParser(nil))
	if err != nil {
		fmt.Fprintf(stderr, "compile plan: %v\n", err)
		return protocol.ExitInternalError
	}
	currentPlanJSON, err := json.MarshalIndent(currentPlan, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "encode current plan: %v\n", err)
		return protocol.ExitInternalError
	}
	currentPlanJSON = append(currentPlanJSON, '\n')

	currentManifest := build.BuildCompileManifest(currentPlan, content, currentPlanJSON, build.DefaultEffectiveConfigHash())

	previousManifest, previousManifestErr := build.ReadCompileManifest(resolvedStateDir)
	if previousManifestErr != nil && !errors.Is(previousManifestErr, os.ErrNotExist) {
		fmt.Fprintf(stderr, "read prior compile manifest: %v\n", previousManifestErr)
		return protocol.ExitInternalError
	}

	requestedSince := strings.TrimSpace(*since)

	var (
		previousPlan    ir.PlanIR
		hasPreviousPlan bool
		advisoryHunks   int
	)
	if requestedSince != "" {
		if !errors.Is(previousManifestErr, os.ErrNotExist) && previousManifest.CompileID == requestedSince {
			previousPlan, hasPreviousPlan, err = readPreviousPlan(resolvedStateDir)
			if err != nil {
				fmt.Fprintf(stderr, "read previous plan state: %v\n", err)
				return protocol.ExitInternalError
			}
			if !hasPreviousPlan {
				fmt.Fprintf(stderr, "requested --since %q but baseline plan snapshot is missing\n", requestedSince)
				return protocol.ExitValidationFailed
			}
		} else {
			baselineContent, err := change.LoadPlanContentAtGitRef(resolvedPlanPath, requestedSince, nil)
			if err != nil {
				if errors.Is(previousManifestErr, os.ErrNotExist) {
					fmt.Fprintf(stderr, "requested --since %q but no prior compile manifest exists and git baseline could not be resolved: %v\n", requestedSince, err)
				} else {
					fmt.Fprintf(stderr, "requested --since %q does not match available baseline %q and git baseline could not be resolved: %v\n", requestedSince, previousManifest.CompileID, err)
				}
				return protocol.ExitValidationFailed
			}
			previousPlan, err = compile.CompilePlan(resolvedPlanPath, baselineContent, compile.NewParser(nil))
			if err != nil {
				fmt.Fprintf(stderr, "compile baseline plan at ref %q: %v\n", requestedSince, err)
				return protocol.ExitInternalError
			}
			hasPreviousPlan = true
			if hints, err := change.LoadPlanGitDiffHintsSince(resolvedPlanPath, requestedSince, nil); err == nil {
				advisoryHunks = len(hints)
			}
		}
	} else {
		previousPlan, hasPreviousPlan, err = readPreviousPlan(resolvedStateDir)
		if err != nil {
			fmt.Fprintf(stderr, "read previous plan state: %v\n", err)
			return protocol.ExitInternalError
		}
	}

	changes := make([]change.TaskChange, 0)
	if hasPreviousPlan {
		changes = change.SemanticDiff(previousPlan, currentPlan)
	} else {
		changes = classifyAllAdded(currentPlan)
	}

	result := changesResult{
		PlanPath:         currentPlan.PlanPath,
		StateDir:         filepath.ToSlash(resolvedStateDir),
		CurrentCompileID: currentManifest.CompileID,
		AdvisoryHunks:    advisoryHunks,
		Changes:          changes,
	}
	if requestedSince != "" {
		if !errors.Is(previousManifestErr, os.ErrNotExist) && previousManifest.CompileID == requestedSince {
			result.SinceCompileID = requestedSince
		} else {
			result.SinceGitRef = requestedSince
		}
	}
	if !errors.Is(previousManifestErr, os.ErrNotExist) {
		result.BaselineCompileID = previousManifest.CompileID
	}

	// Without an explicit baseline constraint, advance state for next change query.
	if requestedSince == "" {
		if _, err := build.WriteCompileManifest(resolvedStateDir, currentManifest); err != nil {
			fmt.Fprintf(stderr, "write compile manifest: %v\n", err)
			return protocol.ExitInternalError
		}
		if err := fsio.WriteFileAtomic(filepath.Join(resolvedStateDir, "build", "plan-latest.json"), currentPlanJSON, 0o644); err != nil {
			fmt.Fprintf(stderr, "write latest plan snapshot: %v\n", err)
			return protocol.ExitInternalError
		}
	}

	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		payload := protocol.Envelope[changesResult]{
			SchemaVersion: protocol.SchemaVersionV01,
			ToolVersion:   CLIVersion,
			Command:       "changes",
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
		fmt.Fprintf(stdout, "current_compile_id: %s\n", result.CurrentCompileID)
		if result.BaselineCompileID != "" {
			fmt.Fprintf(stdout, "baseline_compile_id: %s\n", result.BaselineCompileID)
		}
		if result.SinceCompileID != "" {
			fmt.Fprintf(stdout, "since_compile_id: %s\n", result.SinceCompileID)
		}
		if result.SinceGitRef != "" {
			fmt.Fprintf(stdout, "since_git_ref: %s\n", result.SinceGitRef)
			fmt.Fprintf(stdout, "advisory_hunks: %d\n", result.AdvisoryHunks)
		}
		fmt.Fprintf(stdout, "changes_count: %d\n", len(result.Changes))
		for _, c := range result.Changes {
			fmt.Fprintf(stdout, "- class=%s task_id=%s old_id=%s new_id=%s\n", c.Class, c.TaskID, c.OldID, c.NewID)
		}
	default:
		fmt.Fprintf(stderr, "invalid --format value: %s\n", *format)
		return protocol.ExitUsageError
	}

	return protocol.ExitSuccess
}

func readPreviousPlan(stateDir string) (ir.PlanIR, bool, error) {
	path := filepath.Join(stateDir, "build", "plan-latest.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ir.PlanIR{}, false, nil
		}
		return ir.PlanIR{}, false, err
	}
	var previous ir.PlanIR
	if err := json.Unmarshal(payload, &previous); err != nil {
		return ir.PlanIR{}, false, fmt.Errorf("unmarshal previous plan: %w", err)
	}
	return previous, true, nil
}

func classifyAllAdded(plan ir.PlanIR) []change.TaskChange {
	changes := make([]change.TaskChange, 0, len(plan.Semantic.Tasks))
	for _, task := range plan.Semantic.Tasks {
		id := strings.TrimSpace(task.ID)
		if id == "" {
			continue
		}
		changes = append(changes, change.TaskChange{Class: change.ClassAdded, TaskID: id})
	}
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].TaskID < changes[j].TaskID
	})
	return changes
}
