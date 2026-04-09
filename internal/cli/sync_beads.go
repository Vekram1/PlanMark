package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/compile"
	"github.com/vikramoddiraju/planmark/internal/config"
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
	SeedFromSyncManifest(manifest tracker.SyncManifest)
	PushTask(ctx context.Context, task tracker.TaskProjection) (tracker.PushResult, error)
	WriteSyncManifest(stateDir string) (string, error)
	SetRenderProfile(profile tracker.RenderProfile)
	ValidateTask(task tracker.TaskProjection) error
	Capabilities() tracker.TrackerCapabilities
}

type trackerDBPathConfigurer interface {
	SetDBPath(path string)
}

type syncManifestReconciler interface {
	ReconcileSyncManifest(ctx context.Context, manifest tracker.SyncManifest) (tracker.SyncManifest, error)
}

type staleTaskAdapter interface {
	MarkTaskStale(ctx context.Context, id string, reason string) (tracker.PushResult, error)
}

type dependencySyncAdapter interface {
	SyncDependencies(ctx context.Context, tasks map[string]tracker.TaskProjection) error
}

type staleTrackerCandidateLister interface {
	ListStaleCandidates(ctx context.Context, desiredIDs map[string]struct{}) ([]tracker.SyncManifestEntry, error)
}

type cleanupCandidateLister interface {
	ListCleanupCandidates(ctx context.Context, planPath string, desiredIDs map[string]struct{}) ([]tracker.CleanupCandidate, error)
}

type cleanupCandidateCloser interface {
	CleanupCandidate(ctx context.Context, candidate tracker.CleanupCandidate) error
}

var newBeadsSyncAdapter = func() beadsSyncAdapter {
	return tracker.NewBeadsAdapter()
}

var newGitHubSyncAdapter = func() beadsSyncAdapter {
	return tracker.NewGitHubAdapter()
}

var newLinearSyncAdapter = func() beadsSyncAdapter {
	return tracker.NewLinearAdapter()
}

func runSync(args []string, stdout io.Writer, stderr io.Writer) int {
	target := ""
	filteredArgs := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--plan=") || strings.HasPrefix(arg, "--state-dir=") || strings.HasPrefix(arg, "--format=") || strings.HasPrefix(arg, "--adapter=") || strings.HasPrefix(arg, "--profile=") {
			filteredArgs = append(filteredArgs, arg)
			continue
		}
		if strings.HasPrefix(arg, "--deletion-policy=") {
			filteredArgs = append(filteredArgs, arg)
			continue
		}
		if arg == "--plan" || arg == "--state-dir" || arg == "--format" || arg == "--deletion-policy" || arg == "--adapter" || arg == "--profile" {
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
	planPath := fs.String("plan", "", "path to `plan-file` markdown file")
	stateDir := fs.String("state-dir", ".planmark", "path to local PlanMark `state-dir`")
	adapterFlag := fs.String("adapter", "", "tracker adapter: beads|github|linear")
	profileFlag := fs.String("profile", "", "render profile: default|compact|agentic|handoff")
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
		fmt.Fprintln(stderr, "usage: plan sync [beads|github|linear] --plan <path> [--adapter beads|github|linear] [--profile default|compact|agentic|handoff] [--state-dir <path>] [--deletion-policy mark-stale|close|detach|delete] [--dry-run] [--format text|json]")
		return protocol.ExitUsageError
	}
	if len(remaining) == 1 {
		if target != "" {
			fmt.Fprintln(stderr, "too many positional arguments for sync")
			return protocol.ExitUsageError
		}
		target = remaining[0]
	}
	if strings.TrimSpace(*planPath) == "" {
		fmt.Fprintln(stderr, "missing --plan")
		return protocol.ExitUsageError
	}
	cfgResolved, err := config.LoadForPlan(*planPath)
	if err != nil {
		fmt.Fprintf(stderr, "load config: %v\n", err)
		return protocol.ExitInternalError
	}
	resolvedAdapter, resolvedProfile, err := resolveSyncSelection(strings.TrimSpace(target), strings.TrimSpace(*adapterFlag), strings.TrimSpace(*profileFlag), cfgResolved)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
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
	compileID, err := semanticCompileID(compiled)
	if err != nil {
		fmt.Fprintf(stderr, "compute compile id: %v\n", err)
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

	adapter, err := instantiateSyncAdapter(resolvedAdapter)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitUsageError
	}
	if dbConfigurer, ok := adapter.(trackerDBPathConfigurer); ok {
		dbConfigurer.SetDBPath(filepath.Join(".beads", "beads.db"))
	}
	if resolvedProfile != "" {
		adapter.SetRenderProfile(tracker.RenderProfile(resolvedProfile))
	}
	resolvedStateDir := strings.TrimSpace(*stateDir)
	if resolvedStateDir == "" {
		resolvedStateDir = ".planmark"
	}
	ctx := context.Background()
	priorManifest, err := loadSyncManifest(syncManifestPath(resolvedStateDir, resolvedAdapter))
	if err != nil {
		fmt.Fprintf(stderr, "load sync manifest: %v\n", err)
		return protocol.ExitInternalError
	}
	if reconciler, ok := adapter.(syncManifestReconciler); ok {
		priorManifest, err = reconciler.ReconcileSyncManifest(ctx, priorManifest)
		if err != nil {
			fmt.Fprintf(stderr, "reconcile sync manifest: %v\n", err)
			return protocol.ExitInternalError
		}
	}
	adapter.SeedFromSyncManifest(priorManifest)

	result := syncBeadsResult{
		Target:         resolvedAdapter,
		PlanPath:       *planPath,
		StateDir:       resolvedStateDir,
		DryRun:         *dryRun,
		DeletionPolicy: string(resolvedDeletionPolicy),
	}
	taskProjectionByID := make(map[string]tracker.TaskProjection, len(compiled.Semantic.Tasks))
	desiredIDs := make(map[string]struct{}, len(compiled.Semantic.Tasks))
	desired := make([]syncplanner.DesiredProjection, 0, len(compiled.Semantic.Tasks))
	prior := make([]syncplanner.PriorProjection, 0, len(priorManifest.Entries))

	for _, task := range compiled.Semantic.Tasks {
		node, ok := nodesByRef[task.NodeRef]
		if !ok {
			fmt.Fprintf(stderr, "missing source node for task id %q (node_ref=%s)\n", task.ID, task.NodeRef)
			return protocol.ExitInternalError
		}
		projection := tracker.TaskProjection{
			ID:              task.ID,
			Title:           task.Title,
			CanonicalStatus: task.CanonicalStatus,
			Horizon:         task.Horizon,
			Provenance: tracker.TaskProvenance{
				NodeRef:    task.NodeRef,
				Path:       compiled.PlanPath,
				StartLine:  node.startLine,
				EndLine:    node.endLine,
				SourceHash: node.sliceHash,
				CompileID:  compileID,
			},
			Dependencies: append([]string(nil), task.Deps...),
			Acceptance:   append([]string(nil), task.Accept...),
		}
		if len(task.Sections) > 0 {
			projection.Sections = make([]tracker.TaskProjectionSection, 0, len(task.Sections))
			for _, section := range task.Sections {
				projection.Sections = append(projection.Sections, tracker.TaskProjectionSection{
					Key:   section.Key,
					Title: section.Title,
					Body:  append([]string(nil), section.Body...),
				})
			}
		}
		if len(task.Steps) > 0 {
			projection.Steps = make([]tracker.TaskProjectionStep, 0, len(task.Steps))
			for _, step := range task.Steps {
				projection.Steps = append(projection.Steps, tracker.TaskProjectionStep{
					NodeRef: step.NodeRef,
					Title:   step.Title,
					Checked: step.Checked,
				})
			}
		}
		if len(task.EvidenceNodeRefs) > 0 {
			projection.Evidence = make([]tracker.TaskProjectionEvidence, 0, len(task.EvidenceNodeRefs))
			for _, ref := range task.EvidenceNodeRefs {
				projection.Evidence = append(projection.Evidence, tracker.TaskProjectionEvidence{NodeRef: ref})
			}
		}
		if err := adapter.ValidateTask(projection); err != nil {
			fmt.Fprintf(stderr, "validate task projection: %v\n", err)
			return protocol.ExitInternalError
		}
		hash, err := syncplanner.ProjectionHashForTask(projection)
		if err != nil {
			fmt.Fprintf(stderr, "hash task projection: %v\n", err)
			return protocol.ExitInternalError
		}
		taskProjectionByID[task.ID] = projection
		desiredIDs[task.ID] = struct{}{}
		desired = append(desired, syncplanner.DesiredProjection{
			ID:             task.ID,
			ProjectionHash: hash,
			Provenance: tracker.TaskProvenance{
				NodeRef:    task.NodeRef,
				Path:       compiled.PlanPath,
				StartLine:  node.startLine,
				EndLine:    node.endLine,
				SourceHash: node.sliceHash,
				CompileID:  compileID,
			},
		})
		result.TasksSeen++
	}

	for _, entry := range priorManifest.Entries {
		prior = append(prior, syncplanner.PriorProjection{
			ID:             entry.ID,
			ProjectionHash: entry.ProjectionHash,
			Provenance: tracker.TaskProvenance{
				NodeRef:    entry.NodeRef,
				Path:       entry.SourcePath,
				StartLine:  entry.SourceStartLine,
				EndLine:    entry.SourceEndLine,
				SourceHash: entry.SourceHash,
				CompileID:  entry.CompileID,
			},
		})
	}
	if lister, ok := adapter.(staleTrackerCandidateLister); ok {
		staleCandidates, err := lister.ListStaleCandidates(ctx, desiredIDs)
		if err != nil {
			fmt.Fprintf(stderr, "list stale tracker candidates: %v\n", err)
			return protocol.ExitInternalError
		}
		seenPriorKeys := make(map[string]struct{}, len(prior))
		for _, entry := range prior {
			id := strings.TrimSpace(entry.ID)
			if id == "" {
				continue
			}
			key := id + "\x00" + strings.TrimSpace(entry.ProjectionHash) + "\x00" + strings.TrimSpace(entry.Provenance.NodeRef) + "\x00" + strings.TrimSpace(entry.Provenance.Path)
			seenPriorKeys[key] = struct{}{}
		}
		for _, candidate := range staleCandidates {
			id := strings.TrimSpace(candidate.ID)
			if id == "" {
				continue
			}
			if _, alreadyDesired := desiredIDs[id]; alreadyDesired {
				continue
			}
			key := id + "\x00" + strings.TrimSpace(candidate.RemoteID) + "\x00" + strings.TrimSpace(candidate.ProjectionHash) + "\x00" + strings.TrimSpace(candidate.NodeRef) + "\x00" + strings.TrimSpace(candidate.SourcePath)
			if _, alreadyPrior := seenPriorKeys[key]; alreadyPrior {
				continue
			}
			seenPriorKeys[key] = struct{}{}
			prior = append(prior, syncplanner.PriorProjection{
				ID:             id,
				ProjectionHash: strings.TrimSpace(candidate.ProjectionHash),
				Provenance: tracker.TaskProvenance{
					NodeRef:    candidate.NodeRef,
					Path:       candidate.SourcePath,
					StartLine:  candidate.SourceStartLine,
					EndLine:    candidate.SourceEndLine,
					SourceHash: candidate.SourceHash,
					CompileID:  candidate.CompileID,
				},
			})
		}
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
		if op.Kind == syncplanner.OperationMarkStale {
			handler, ok := adapter.(staleTaskAdapter)
			if !ok {
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
			var staleResult tracker.PushResult
			var staleErr error
			for attempt := 1; ; attempt++ {
				staleResult, staleErr = handler.MarkTaskStale(ctx, op.ID, op.Reason)
				if staleErr == nil {
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

				retryable := tracker.IsRetryableSyncError(staleErr)
				maxAttempts := retryMaxAttempts(staleErr)
				errDetail := staleErr.Error()
				if retryable && attempt < maxAttempts {
					errDetail = fmt.Sprintf("%s (retry scheduled: backoff_ms=%d)", errDetail, retryBackoffMillis(staleErr, attempt))
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
					fmt.Fprintf(stderr, "mark stale task projection: %v\n", staleErr)
					return protocol.ExitInternalError
				}
			}
			if staleResult.Mutated {
				result.TasksMutated++
			}
			if strings.Contains(staleResult.Diagnostic, "drift detected") {
				result.DriftCount++
			}
			continue
		}
		if op.Kind != syncplanner.OperationCreate && op.Kind != syncplanner.OperationUpdate && op.Kind != syncplanner.OperationNoop {
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
		if result.ConflictCount == 0 {
			if depSync, ok := adapter.(dependencySyncAdapter); ok {
				if err := depSync.SyncDependencies(ctx, taskProjectionByID); err != nil {
					fmt.Fprintf(stderr, "sync task dependencies: %v\n", err)
					return protocol.ExitInternalError
				}
			}
		}
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
			Command:       "sync " + resolvedAdapter,
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

func semanticCompileID(compiled any) (string, error) {
	payload, err := json.Marshal(compiled)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func resolveSyncSelection(target string, adapterFlag string, profileFlag string, cfg config.Resolved) (string, string, error) {
	resolvedAdapter := adapterFlag
	if resolvedAdapter == "" {
		resolvedAdapter = target
	}
	if resolvedAdapter == "" {
		resolvedAdapter = strings.TrimSpace(cfg.Tracker.Adapter)
	}
	if resolvedAdapter == "" {
		resolvedAdapter = "beads"
	}
	if adapterFlag != "" && target != "" && adapterFlag != target {
		return "", "", fmt.Errorf("--adapter %q conflicts with positional target %q", adapterFlag, target)
	}
	switch resolvedAdapter {
	case "beads", "github", "linear":
	default:
		return "", "", fmt.Errorf("unsupported sync adapter %q", resolvedAdapter)
	}
	resolvedProfile := profileFlag
	if resolvedProfile == "" {
		resolvedProfile = strings.TrimSpace(cfg.Tracker.Profile)
	}
	if resolvedProfile != "" {
		switch tracker.RenderProfile(strings.ToLower(resolvedProfile)) {
		case tracker.RenderProfileDefault, tracker.RenderProfileCompact, tracker.RenderProfileAgentic, tracker.RenderProfileHandoff:
			resolvedProfile = strings.ToLower(resolvedProfile)
		default:
			return "", "", fmt.Errorf("unsupported render profile %q", resolvedProfile)
		}
	}
	return resolvedAdapter, resolvedProfile, nil
}

func instantiateSyncAdapter(name string) (beadsSyncAdapter, error) {
	switch strings.TrimSpace(name) {
	case "beads":
		return newBeadsSyncAdapter(), nil
	case "github":
		return newGitHubSyncAdapter(), nil
	case "linear":
		return newLinearSyncAdapter(), nil
	default:
		return nil, fmt.Errorf("unsupported sync adapter %q", name)
	}
}

func syncManifestPath(stateDir string, adapterName string) string {
	fileName := adapterName + "-manifest.json"
	if strings.TrimSpace(adapterName) == "beads" {
		fileName = "beads-manifest.json"
	}
	return filepath.Join(stateDir, "sync", fileName)
}

func loadSyncManifest(path string) (tracker.SyncManifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return tracker.SyncManifest{
				SchemaVersion: tracker.SyncManifestSchemaVersionV01,
				Entries:       nil,
			}, nil
		}
		return tracker.SyncManifest{}, err
	}

	var manifest tracker.SyncManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return tracker.SyncManifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	if strings.TrimSpace(manifest.SchemaVersion) == "" {
		return tracker.SyncManifest{}, fmt.Errorf("decode manifest: missing schema_version")
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
