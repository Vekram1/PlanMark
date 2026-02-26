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
	"strings"

	"github.com/vikramoddiraju/planmark/internal/compile"
	"github.com/vikramoddiraju/planmark/internal/journal"
	"github.com/vikramoddiraju/planmark/internal/protocol"
	"github.com/vikramoddiraju/planmark/internal/syncplanner"
	"github.com/vikramoddiraju/planmark/internal/tracker"
)

type syncBeadsResult struct {
	Target         string                 `json:"target"`
	PlanPath       string                 `json:"plan_path"`
	StateDir       string                 `json:"state_dir,omitempty"`
	DryRun         bool                   `json:"dry_run"`
	DeletionPolicy string                 `json:"deletion_policy"`
	TasksSeen      int                    `json:"tasks_seen"`
	TasksMutated   int                    `json:"tasks_mutated"`
	DriftCount     int                    `json:"drift_count"`
	ManifestPath   string                 `json:"manifest_path,omitempty"`
	CreateCount    int                    `json:"create_count"`
	UpdateCount    int                    `json:"update_count"`
	NoopCount      int                    `json:"noop_count"`
	MarkStaleCount int                    `json:"mark_stale_count"`
	ConflictCount  int                    `json:"conflict_count"`
	PlannedOps     []syncPreviewOperation `json:"planned_ops,omitempty"`
}

type syncPreviewOperation struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

type beadsSyncAdapter interface {
	SeedFromSyncManifest(manifest tracker.BeadsSyncManifest)
	PushTask(ctx context.Context, task tracker.TaskProjection) (tracker.PushResult, error)
	WriteSyncManifest(stateDir string) (string, error)
}

var newBeadsSyncAdapter = func() beadsSyncAdapter {
	return tracker.NewBeadsAdapter()
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
		if strings.HasPrefix(arg, "--deletion-policy=") {
			filteredArgs = append(filteredArgs, arg)
			continue
		}
		if arg == "--plan" || arg == "--state-dir" || arg == "--format" || arg == "--deletion-policy" {
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
	deletionPolicy := fs.String("deletion-policy", string(syncplanner.DefaultDeletionPolicy()), "deletion policy for PLAN removals: mark-stale|close|detach|delete")
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
		fmt.Fprintln(stderr, "usage: plan sync beads --plan <path> [--state-dir <path>] [--deletion-policy mark-stale|close|detach|delete] [--dry-run] [--format text|json]")
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
		fmt.Fprintln(stderr, "usage: plan sync beads --plan <path> [--state-dir <path>] [--deletion-policy mark-stale|close|detach|delete] [--dry-run] [--format text|json]")
		return protocol.ExitUsageError
	}
	if strings.TrimSpace(*planPath) == "" {
		fmt.Fprintln(stderr, "missing --plan")
		return protocol.ExitUsageError
	}
	resolvedDeletionPolicy, err := syncplanner.ParseDeletionPolicy(*deletionPolicy)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
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

	adapter := newBeadsSyncAdapter()
	resolvedStateDir := strings.TrimSpace(*stateDir)
	if resolvedStateDir == "" {
		resolvedStateDir = ".planmark"
	}
	priorManifest, err := loadBeadsManifest(filepath.Join(resolvedStateDir, "sync", "beads-manifest.json"))
	if err != nil {
		fmt.Fprintf(stderr, "load sync manifest: %v\n", err)
		return protocol.ExitInternalError
	}
	adapter.SeedFromSyncManifest(priorManifest)

	result := syncBeadsResult{
		Target:         "beads",
		PlanPath:       *planPath,
		StateDir:       resolvedStateDir,
		DryRun:         *dryRun,
		DeletionPolicy: string(resolvedDeletionPolicy),
	}
	taskProjectionByID := make(map[string]tracker.TaskProjection, len(compiled.Semantic.Tasks))
	desired := make([]syncplanner.DesiredProjection, 0, len(compiled.Semantic.Tasks))
	prior := make([]syncplanner.PriorProjection, 0, len(priorManifest.Entries))

	ctx := context.Background()
	for _, task := range compiled.Semantic.Tasks {
		node, ok := nodesByRef[task.NodeRef]
		if !ok {
			fmt.Fprintf(stderr, "missing source node for task id %q (node_ref=%s)\n", task.ID, task.NodeRef)
			return protocol.ExitInternalError
		}
		projection := tracker.TaskProjection{
			ID:              task.ID,
			Title:           task.Title,
			SourcePath:      compiled.PlanPath,
			SourceStartLine: node.startLine,
			SourceEndLine:   node.endLine,
			SourceHash:      node.sliceHash,
			Accept:          append([]string(nil), task.Accept...),
		}
		hash, err := syncplanner.ProjectionHashForTask(projection)
		if err != nil {
			fmt.Fprintf(stderr, "hash task projection: %v\n", err)
			return protocol.ExitInternalError
		}
		taskProjectionByID[task.ID] = projection
		desired = append(desired, syncplanner.DesiredProjection{ID: task.ID, ProjectionHash: hash})
		result.TasksSeen++
	}

	for _, entry := range priorManifest.Entries {
		prior = append(prior, syncplanner.PriorProjection{ID: entry.ID, ProjectionHash: entry.ProjectionHash})
	}

	ops := syncplanner.PlanSyncOps(desired, prior, resolvedDeletionPolicy)
	if *dryRun {
		result.PlannedOps = make([]syncPreviewOperation, 0, len(ops))
	}
	for _, op := range ops {
		opID := journal.SyncOperationID(op)
		if *dryRun {
			result.PlannedOps = append(result.PlannedOps, syncPreviewOperation{
				Kind:   string(op.Kind),
				ID:     op.ID,
				Reason: op.Reason,
			})
		}
		switch op.Kind {
		case syncplanner.OperationCreate:
			result.CreateCount++
		case syncplanner.OperationUpdate:
			result.UpdateCount++
		case syncplanner.OperationNoop:
			result.NoopCount++
		case syncplanner.OperationMarkStale:
			result.MarkStaleCount++
		case syncplanner.OperationConflict:
			result.ConflictCount++
		}
		if err := journal.AppendAttempt(resolvedStateDir, journal.OperationAttempt{
			OperationID: opID,
			Kind:        string(op.Kind),
			ID:          op.ID,
			Attempt:     1,
			Outcome:     journal.OutcomePlanned,
		}); err != nil {
			fmt.Fprintf(stderr, "append sync journal planned record: %v\n", err)
			return protocol.ExitInternalError
		}
	}

	for _, op := range ops {
		opID := journal.SyncOperationID(op)
		if *dryRun {
			if err := journal.AppendAttempt(resolvedStateDir, journal.OperationAttempt{
				OperationID: opID,
				Kind:        string(op.Kind),
				ID:          op.ID,
				Attempt:     1,
				Outcome:     journal.OutcomeSkipped,
			}); err != nil {
				fmt.Fprintf(stderr, "append sync journal dry-run skip: %v\n", err)
				return protocol.ExitInternalError
			}
			continue
		}
		if op.Kind != syncplanner.OperationCreate && op.Kind != syncplanner.OperationUpdate {
			if err := journal.AppendAttempt(resolvedStateDir, journal.OperationAttempt{
				OperationID: opID,
				Kind:        string(op.Kind),
				ID:          op.ID,
				Attempt:     1,
				Outcome:     journal.OutcomeSkipped,
			}); err != nil {
				fmt.Fprintf(stderr, "append sync journal non-mutating skip: %v\n", err)
				return protocol.ExitInternalError
			}
			continue
		}
		taskProjection, ok := taskProjectionByID[op.ID]
		if !ok {
			if err := journal.AppendAttempt(resolvedStateDir, journal.OperationAttempt{
				OperationID: opID,
				Kind:        string(op.Kind),
				ID:          op.ID,
				Attempt:     1,
				Outcome:     journal.OutcomeFailed,
				Error:       "missing task projection for planned operation",
			}); err != nil {
				fmt.Fprintf(stderr, "append sync journal missing projection failure: %v\n", err)
				return protocol.ExitInternalError
			}
			continue
		}
		var pushResult tracker.PushResult
		var pushErr error
		for attempt := 1; ; attempt++ {
			pushResult, pushErr = adapter.PushTask(ctx, taskProjection)
			if pushErr == nil {
				if err := journal.AppendAttempt(resolvedStateDir, journal.OperationAttempt{
					OperationID: opID,
					Kind:        string(op.Kind),
					ID:          op.ID,
					Attempt:     attempt,
					Outcome:     journal.OutcomeSuccess,
				}); err != nil {
					fmt.Fprintf(stderr, "append sync journal success record: %v\n", err)
					return protocol.ExitInternalError
				}
				break
			}

			retryable := tracker.IsRetryableSyncError(pushErr)
			maxAttempts := retryMaxAttempts(pushErr)
			errDetail := pushErr.Error()
			if retryable && attempt < maxAttempts {
				errDetail = fmt.Sprintf("%s (retry scheduled: backoff_ms=%d)", errDetail, retryBackoffMillis(pushErr, attempt))
			}
			if err := journal.AppendAttempt(resolvedStateDir, journal.OperationAttempt{
				OperationID: opID,
				Kind:        string(op.Kind),
				ID:          op.ID,
				Attempt:     attempt,
				Outcome:     journal.OutcomeFailed,
				Error:       errDetail,
			}); err != nil {
				fmt.Fprintf(stderr, "append sync journal failure record: %v\n", err)
				return protocol.ExitInternalError
			}
			if !retryable || attempt >= maxAttempts {
				fmt.Fprintf(stderr, "push task projection: %v\n", pushErr)
				return protocol.ExitInternalError
			}
		}
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
		fmt.Fprintf(stdout, "deletion_policy: %s\n", result.DeletionPolicy)
		fmt.Fprintf(stdout, "tasks_seen: %d\n", result.TasksSeen)
		fmt.Fprintf(stdout, "tasks_mutated: %d\n", result.TasksMutated)
		fmt.Fprintf(stdout, "drift_count: %d\n", result.DriftCount)
		fmt.Fprintf(stdout, "create_count: %d\n", result.CreateCount)
		fmt.Fprintf(stdout, "update_count: %d\n", result.UpdateCount)
		fmt.Fprintf(stdout, "noop_count: %d\n", result.NoopCount)
		fmt.Fprintf(stdout, "mark_stale_count: %d\n", result.MarkStaleCount)
		fmt.Fprintf(stdout, "conflict_count: %d\n", result.ConflictCount)
		if result.DryRun {
			fmt.Fprintln(stdout, "planned_ops:")
			for _, op := range result.PlannedOps {
				fmt.Fprintf(stdout, "- %s %s (%s)\n", op.Kind, op.ID, op.Reason)
			}
		}
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

func loadBeadsManifest(path string) (tracker.BeadsSyncManifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return tracker.BeadsSyncManifest{
				SchemaVersion: tracker.BeadsManifestSchemaVersionV01,
				Entries:       nil,
			}, nil
		}
		return tracker.BeadsSyncManifest{}, err
	}

	var manifest tracker.BeadsSyncManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return tracker.BeadsSyncManifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	if strings.TrimSpace(manifest.SchemaVersion) == "" {
		return tracker.BeadsSyncManifest{}, fmt.Errorf("decode manifest: missing schema_version")
	}
	return manifest, nil
}

func retryMaxAttempts(err error) int {
	if tracker.IsRateLimitedSyncError(err) {
		return 4
	}
	if tracker.IsTransientSyncError(err) {
		return 3
	}
	return 1
}

func retryBackoffMillis(err error, attempt int) int {
	base := 100
	if tracker.IsRateLimitedSyncError(err) {
		base = 250
	}
	if attempt <= 0 {
		attempt = 1
	}
	return base * attempt
}
