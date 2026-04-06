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
	"github.com/vikramoddiraju/planmark/internal/ir"
	"github.com/vikramoddiraju/planmark/internal/protocol"
)

type applyChangeResult struct {
	PlanPath          string `json:"plan_path"`
	DeltaPath         string `json:"delta_path"`
	BasePlanHash      string `json:"base_plan_hash"`
	UpdatedPlanHash   string `json:"updated_plan_hash"`
	OperationsApplied int    `json:"operations_applied"`
	DoctorDiagnostics int    `json:"doctor_diagnostics"`
}

func runApplyChange(args []string, stdout io.Writer, stderr io.Writer) int {
	positionalDelta := ""
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
		if positionalDelta == "" {
			positionalDelta = arg
			continue
		}
		filteredArgs = append(filteredArgs, arg)
	}

	fs := flag.NewFlagSet("apply-change", flag.ContinueOnError)
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
		fmt.Fprintln(stderr, "usage: plan apply-change <delta-file> --plan <path> [--format text|json]")
		return protocol.ExitUsageError
	}
	if len(remaining) == 1 {
		if positionalDelta != "" {
			fmt.Fprintln(stderr, "too many positional arguments for apply-change")
			return protocol.ExitUsageError
		}
		positionalDelta = remaining[0]
	}
	if strings.TrimSpace(positionalDelta) == "" {
		fmt.Fprintln(stderr, "usage: plan apply-change <delta-file> --plan <path> [--format text|json]")
		return protocol.ExitUsageError
	}
	if strings.TrimSpace(*planPath) == "" {
		fmt.Fprintln(stderr, "missing --plan")
		return protocol.ExitUsageError
	}

	deltaRaw, err := os.ReadFile(positionalDelta)
	if err != nil {
		fmt.Fprintf(stderr, "read delta: %v\n", err)
		return protocol.ExitInternalError
	}
	var delta planDeltaDocument
	if err := json.Unmarshal(deltaRaw, &delta); err != nil {
		fmt.Fprintf(stderr, "decode delta: %v\n", err)
		return protocol.ExitValidationFailed
	}
	if delta.SchemaVersion != planDeltaSchemaVersionV01 {
		fmt.Fprintf(stderr, "unsupported delta schema version: %s\n", delta.SchemaVersion)
		return protocol.ExitValidationFailed
	}

	planRaw, err := os.ReadFile(*planPath)
	if err != nil {
		fmt.Fprintf(stderr, "read plan: %v\n", err)
		return protocol.ExitInternalError
	}
	baseHash := sha256Hex(planRaw)
	if strings.TrimSpace(delta.BasePlanHash) != baseHash {
		fmt.Fprintf(stderr, "base_hash_mismatch: delta=%s current=%s\n", strings.TrimSpace(delta.BasePlanHash), baseHash)
		return protocol.ExitValidationFailed
	}

	updated := normalizeLF(planRaw)
	opsApplied := 0
	for _, op := range delta.Operations {
		var applyErr error
		updated, applyErr = applyDeltaOperation(updated, *planPath, op)
		if applyErr != nil {
			fmt.Fprintf(stderr, "%v\n", applyErr)
			return protocol.ExitValidationFailed
		}
		opsApplied++
	}

	if err := os.WriteFile(*planPath, []byte(updated), 0o644); err != nil {
		fmt.Fprintf(stderr, "write plan: %v\n", err)
		return protocol.ExitInternalError
	}

	compiled, err := compile.CompilePlan(*planPath, []byte(updated), compile.NewParser(nil))
	if err != nil {
		fmt.Fprintf(stderr, "recompile after apply: %v\n", err)
		return protocol.ExitInternalError
	}
	doctorResult, err := doctor.Run(compiled, "build")
	if err != nil {
		fmt.Fprintf(stderr, "doctor after apply: %v\n", err)
		return protocol.ExitInternalError
	}

	result := applyChangeResult{
		PlanPath:          compiled.PlanPath,
		DeltaPath:         positionalDelta,
		BasePlanHash:      baseHash,
		UpdatedPlanHash:   sha256Hex([]byte(updated)),
		OperationsApplied: opsApplied,
		DoctorDiagnostics: len(doctorResult.Diagnostics),
	}

	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		payload := protocol.Envelope[applyChangeResult]{
			SchemaVersion: protocol.SchemaVersionV01,
			ToolVersion:   CLIVersion,
			Command:       "apply-change",
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
		fmt.Fprintf(stdout, "delta_path: %s\n", result.DeltaPath)
		fmt.Fprintf(stdout, "base_plan_hash: %s\n", result.BasePlanHash)
		fmt.Fprintf(stdout, "updated_plan_hash: %s\n", result.UpdatedPlanHash)
		fmt.Fprintf(stdout, "operations_applied: %d\n", result.OperationsApplied)
		fmt.Fprintf(stdout, "doctor_diagnostics: %d\n", result.DoctorDiagnostics)
	default:
		fmt.Fprintf(stderr, "invalid --format value: %s\n", *format)
		return protocol.ExitUsageError
	}
	return protocol.ExitSuccess
}

func applyDeltaOperation(planText string, planPath string, op planDeltaOperation) (string, error) {
	if strings.TrimSpace(op.Kind) != "metadata_upsert" {
		return "", fmt.Errorf("operation_kind_unsupported: %s", op.Kind)
	}
	if strings.TrimSpace(op.Payload.Key) == "" {
		return "", fmt.Errorf("operation payload key is required")
	}

	compiled, err := compile.CompilePlan(planPath, []byte(planText), compile.NewParser(nil))
	if err != nil {
		return "", fmt.Errorf("compile before apply op %s: %w", op.OpID, err)
	}
	_, node, err := resolveDeltaTarget(compiled, op.Target)
	if err != nil {
		return "", err
	}

	if node.StartLine != op.Precondition.SourceRange.StartLine || node.EndLine != op.Precondition.SourceRange.EndLine {
		return "", fmt.Errorf("precondition_range_mismatch: op=%s expected=%d-%d actual=%d-%d", op.OpID, op.Precondition.SourceRange.StartLine, op.Precondition.SourceRange.EndLine, node.StartLine, node.EndLine)
	}
	if strings.TrimSpace(op.Precondition.SourceHash) != strings.TrimSpace(node.SliceHash) {
		return "", fmt.Errorf("precondition_hash_mismatch: op=%s expected=%s actual=%s", op.OpID, strings.TrimSpace(op.Precondition.SourceHash), strings.TrimSpace(node.SliceHash))
	}

	lines := strings.Split(planText, "\n")
	insertIndex := node.EndLine
	if insertIndex < 0 {
		insertIndex = 0
	}
	if insertIndex > len(lines) {
		insertIndex = len(lines)
	}
	metadataLine := fmt.Sprintf("  @%s %s", strings.TrimSpace(op.Payload.Key), strings.TrimSpace(op.Payload.Value))
	lines = append(lines[:insertIndex], append([]string{metadataLine}, lines[insertIndex:]...)...)
	return strings.Join(lines, "\n"), nil
}

func resolveDeltaTarget(plan ir.PlanIR, target planDeltaTarget) (ir.Task, ir.SourceNode, error) {
	if strings.TrimSpace(target.NodeRef) != "" {
		nodeRef := strings.TrimSpace(target.NodeRef)
		for _, task := range plan.Semantic.Tasks {
			if strings.TrimSpace(task.NodeRef) != nodeRef {
				continue
			}
			for _, node := range plan.Source.Nodes {
				if strings.TrimSpace(node.NodeRef) == nodeRef {
					return task, node, nil
				}
			}
			return ir.Task{}, ir.SourceNode{}, fmt.Errorf("target_unresolved: node_ref %s has no source node", nodeRef)
		}
		return ir.Task{}, ir.SourceNode{}, fmt.Errorf("target_unresolved: node_ref %s", nodeRef)
	}

	if strings.TrimSpace(target.TaskID) != "" {
		taskID := strings.TrimSpace(target.TaskID)
		matches := make([]ir.Task, 0, 1)
		for _, task := range plan.Semantic.Tasks {
			if strings.TrimSpace(task.ID) == taskID {
				matches = append(matches, task)
			}
		}
		if len(matches) == 0 {
			return ir.Task{}, ir.SourceNode{}, fmt.Errorf("target_unresolved: task_id %s", taskID)
		}
		if len(matches) > 1 {
			return ir.Task{}, ir.SourceNode{}, fmt.Errorf("target_ambiguous: task_id %s", taskID)
		}
		task := matches[0]
		for _, node := range plan.Source.Nodes {
			if strings.TrimSpace(node.NodeRef) == strings.TrimSpace(task.NodeRef) {
				return task, node, nil
			}
		}
		return ir.Task{}, ir.SourceNode{}, fmt.Errorf("target_unresolved: source node missing for task_id %s", taskID)
	}

	return ir.Task{}, ir.SourceNode{}, fmt.Errorf("target_unresolved: missing node_ref/task_id")
}

func normalizeLF(raw []byte) string {
	content := string(raw)
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	return content
}
