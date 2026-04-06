package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/vikramoddiraju/planmark/internal/accept"
	"github.com/vikramoddiraju/planmark/internal/compile"
	"github.com/vikramoddiraju/planmark/internal/ir"
	"github.com/vikramoddiraju/planmark/internal/protocol"
)

func runVerifyAccept(args []string, stdout io.Writer, stderr io.Writer) int {
	taskID := ""
	filteredArgs := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--plan=") || strings.HasPrefix(arg, "--accept-index=") || strings.HasPrefix(arg, "--timeout-ms=") || strings.HasPrefix(arg, "--format=") {
			filteredArgs = append(filteredArgs, arg)
			continue
		}
		if arg == "--plan" || arg == "--accept-index" || arg == "--timeout-ms" || arg == "--format" {
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
		if taskID == "" {
			taskID = arg
			continue
		}
		filteredArgs = append(filteredArgs, arg)
	}

	fs := flag.NewFlagSet("verify-accept", flag.ContinueOnError)
	fs.SetOutput(stderr)
	planPath := fs.String("plan", "", "path to `plan-file` markdown file")
	acceptIndex := fs.Int("accept-index", 0, "index of @accept command to execute")
	timeoutMS := fs.Int("timeout-ms", 60000, "command timeout in milliseconds")
	format := fs.String("format", "json", "output format: json|text")
	if err := fs.Parse(filteredArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return protocol.ExitSuccess
		}
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitUsageError
	}

	remaining := fs.Args()
	if len(remaining) > 1 {
		fmt.Fprintln(stderr, "usage: plan verify-accept <task-id> [--plan <path>] [--accept-index N] [--timeout-ms N] [--format json|text]")
		return protocol.ExitUsageError
	}
	if len(remaining) == 1 {
		if taskID != "" {
			fmt.Fprintln(stderr, "usage: plan verify-accept <task-id> [--plan <path>] [--accept-index N] [--timeout-ms N] [--format json|text]")
			return protocol.ExitUsageError
		}
		taskID = remaining[0]
	}
	if taskID == "" {
		fmt.Fprintln(stderr, "usage: plan verify-accept <task-id> [--plan <path>] [--accept-index N] [--timeout-ms N] [--format json|text]")
		return protocol.ExitUsageError
	}
	if *planPath == "" {
		fmt.Fprintln(stderr, "missing --plan path")
		return protocol.ExitUsageError
	}
	if *acceptIndex < 0 {
		fmt.Fprintln(stderr, "--accept-index must be >= 0")
		return protocol.ExitUsageError
	}
	if *timeoutMS <= 0 {
		fmt.Fprintln(stderr, "--timeout-ms must be > 0")
		return protocol.ExitUsageError
	}

	compiled, code := compileFromPlanPath(*planPath, stderr)
	if code != protocol.ExitSuccess {
		return code
	}

	task, err := findTaskByID(compiled, taskID)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitValidationFailed
	}
	if len(task.Accept) == 0 {
		fmt.Fprintf(stderr, "task %q has no @accept entries\n", taskID)
		return protocol.ExitValidationFailed
	}
	if *acceptIndex >= len(task.Accept) {
		fmt.Fprintf(stderr, "accept index %d out of range (task has %d @accept entries)\n", *acceptIndex, len(task.Accept))
		return protocol.ExitValidationFailed
	}

	acceptText := task.Accept[*acceptIndex]
	commandText, err := accept.ParseCommandText(acceptText)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitValidationFailed
	}

	planAbs, err := filepath.Abs(compiled.PlanPath)
	if err != nil {
		fmt.Fprintf(stderr, "resolve plan path: %v\n", err)
		return protocol.ExitInternalError
	}
	repoRoot, cwdPolicy := resolveExecutionRoot(planAbs)
	startedAt := time.Now().UTC()

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeoutMS)*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", commandText)
	cmd.Dir = repoRoot
	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	runErr := cmd.Run()
	finishedAt := time.Now().UTC()
	durationMS := finishedAt.Sub(startedAt).Milliseconds()

	exitStatus := 0
	status := "ok"
	switch {
	case runErr == nil:
		status = "ok"
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		status = "timeout"
		exitStatus = -1
	default:
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			status = "failed"
			if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				exitStatus = ws.ExitStatus()
			} else {
				exitStatus = 1
			}
		} else {
			status = "internal_error"
			exitStatus = 1
		}
	}

	receipt := accept.VerificationReceipt{
		SchemaVersion: accept.ReceiptSchemaVersion,
		ToolVersion:   CLIVersion,
		ReceiptID:     accept.NewReceiptID(taskID, *acceptIndex, acceptText, startedAt),
		CreatedAt:     startedAt.Format(time.RFC3339),
		Command: accept.CommandRecord{
			TaskID:        taskID,
			AcceptIndex:   *acceptIndex,
			AcceptText:    acceptText,
			CommandText:   commandText,
			CommandDigest: accept.NewDigest(commandText),
		},
		Policy: accept.PolicyRecord{
			PolicyVersion: accept.PolicyVersion,
			CWDPolicy:     cwdPolicy,
			EnvPolicy:     "inherited",
			TimeoutMS:     *timeoutMS,
			NetworkPolicy: "inherited",
			SandboxPolicy: "host",
		},
		Result: accept.ResultRecord{
			ExitStatus: exitStatus,
			StartedAt:  startedAt.Format(time.RFC3339),
			FinishedAt: finishedAt.Format(time.RFC3339),
			DurationMS: durationMS,
			Status:     status,
		},
		Capture: accept.CaptureRecord{
			Stdout: accept.NewCaptureStream(stdoutBuf.Bytes()),
			Stderr: accept.NewCaptureStream(stderrBuf.Bytes()),
			Redaction: accept.RedactionRecord{
				Applied: false,
			},
		},
		Context: accept.ContextRecord{
			RepoRoot: filepath.ToSlash(repoRoot),
			PlanPath: filepath.ToSlash(planAbs),
			StateDir: "",
		},
	}

	envelope := protocol.Envelope[accept.VerificationReceipt]{
		SchemaVersion: protocol.SchemaVersionV01,
		ToolVersion:   CLIVersion,
		Command:       "verify-accept",
		Status:        status,
		Data:          receipt,
	}

	switch strings.ToLower(*format) {
	case "json":
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(envelope); err != nil {
			fmt.Fprintf(stderr, "encode receipt: %v\n", err)
			return protocol.ExitInternalError
		}
	case "text":
		fmt.Fprintf(stdout, "verify-accept %s\n", status)
		fmt.Fprintf(stdout, "task: %s\n", taskID)
		fmt.Fprintf(stdout, "accept-index: %d\n", *acceptIndex)
		fmt.Fprintf(stdout, "command: %s\n", commandText)
		fmt.Fprintf(stdout, "exit-status: %d\n", exitStatus)
		fmt.Fprintf(stdout, "receipt-id: %s\n", receipt.ReceiptID)
	default:
		fmt.Fprintf(stderr, "invalid --format value: %s\n", *format)
		return protocol.ExitUsageError
	}

	if status == "ok" {
		return protocol.ExitSuccess
	}
	if status == "failed" || status == "timeout" || status == "policy_blocked" {
		return protocol.ExitValidationFailed
	}
	return protocol.ExitInternalError
}

func compileFromPlanPath(planPath string, stderr io.Writer) (ir.PlanIR, int) {
	data, err := os.ReadFile(planPath)
	if err != nil {
		fmt.Fprintf(stderr, "read plan: %v\n", err)
		return ir.PlanIR{}, protocol.ExitInternalError
	}
	compiled, err := compile.CompilePlan(planPath, data, compile.NewParser(nil))
	if err != nil {
		fmt.Fprintf(stderr, "compile plan: %v\n", err)
		return ir.PlanIR{}, protocol.ExitInternalError
	}
	return compiled, protocol.ExitSuccess
}

func findTaskByID(compiled ir.PlanIR, taskID string) (ir.Task, error) {
	for _, task := range compiled.Semantic.Tasks {
		if task.ID == taskID {
			return task, nil
		}
	}
	return ir.Task{}, fmt.Errorf("task not found: %s", taskID)
}

func resolveExecutionRoot(planAbs string) (root string, policy string) {
	start := filepath.Dir(planAbs)
	cur := start
	for {
		gitPath := filepath.Join(cur, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			return cur, "repo-root"
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return start, "plan-dir"
}
