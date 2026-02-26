package cli

import (
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
	contextpkg "github.com/vikramoddiraju/planmark/internal/context"
	"github.com/vikramoddiraju/planmark/internal/protocol"
)

const planDeltaSchemaVersionV01 = "v0.1"

type proposeChangeResult struct {
	TaskID        string            `json:"task_id"`
	PlanPath      string            `json:"plan_path"`
	BasePlanHash  string            `json:"base_plan_hash"`
	Delta         planDeltaDocument `json:"delta"`
	SuggestedDiff []string          `json:"suggested_diff,omitempty"`
}

type planDeltaDocument struct {
	SchemaVersion string               `json:"schema_version"`
	BasePlanHash  string               `json:"base_plan_hash"`
	Operations    []planDeltaOperation `json:"operations"`
}

type planDeltaOperation struct {
	OpID         string             `json:"op_id"`
	Kind         string             `json:"kind"`
	Target       planDeltaTarget    `json:"target"`
	Precondition planDeltaCondition `json:"precondition"`
	Payload      planDeltaPayload   `json:"payload"`
}

type planDeltaTarget struct {
	NodeRef string `json:"node_ref,omitempty"`
	TaskID  string `json:"task_id,omitempty"`
	Path    string `json:"path,omitempty"`
}

type planDeltaCondition struct {
	SourceHash  string             `json:"source_hash"`
	SourceRange planDeltaLineRange `json:"source_range"`
}

type planDeltaLineRange struct {
	StartLine int `json:"start_line"`
	EndLine   int `json:"end_line"`
}

type planDeltaPayload struct {
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"`
}

func runProposeChange(args []string, stdout io.Writer, stderr io.Writer) int {
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

	fs := flag.NewFlagSet("propose-change", flag.ContinueOnError)
	fs.SetOutput(stderr)
	planPath := fs.String("plan", "", "path to PLAN markdown file")
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
		fmt.Fprintln(stderr, "usage: plan propose-change <id> --plan <path> [--format text|json]")
		return protocol.ExitUsageError
	}
	if len(remaining) == 1 {
		if positionalID != "" {
			fmt.Fprintln(stderr, "too many positional arguments for propose-change")
			return protocol.ExitUsageError
		}
		positionalID = remaining[0]
	}
	if strings.TrimSpace(positionalID) == "" {
		fmt.Fprintln(stderr, "usage: plan propose-change <id> --plan <path> [--format text|json]")
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
	explain, err := contextpkg.Explain(compiled, taskID)
	if err != nil {
		if errors.Is(err, contextpkg.ErrTaskNotFound) {
			fmt.Fprintln(stderr, err.Error())
			return protocol.ExitValidationFailed
		}
		fmt.Fprintf(stderr, "explain task: %v\n", err)
		return protocol.ExitInternalError
	}
	openResult, err := contextpkg.Open(compiled, taskID)
	if err != nil {
		if errors.Is(err, contextpkg.ErrTaskNotFound) {
			fmt.Fprintln(stderr, err.Error())
			return protocol.ExitValidationFailed
		}
		fmt.Fprintf(stderr, "open task: %v\n", err)
		return protocol.ExitInternalError
	}

	basePlanHash := sha256Hex(content)
	operations := make([]planDeltaOperation, 0, len(explain.Blockers))
	suggestedDiff := make([]string, 0, 1)
	for _, blocker := range explain.Blockers {
		if blocker.Code != "MISSING_ACCEPT" {
			continue
		}
		operations = append(operations, planDeltaOperation{
			OpID: "op-001",
			Kind: "metadata_upsert",
			Target: planDeltaTarget{
				NodeRef: openResult.NodeRef,
				TaskID:  taskID,
				Path:    compiled.PlanPath,
			},
			Precondition: planDeltaCondition{
				SourceHash: openResult.SliceHash,
				SourceRange: planDeltaLineRange{
					StartLine: openResult.StartLine,
					EndLine:   openResult.EndLine,
				},
			},
			Payload: planDeltaPayload{
				Key:   "accept",
				Value: "cmd:<command>",
			},
		})
		suggestedDiff = append(suggestedDiff, fmt.Sprintf("%s:%d +  @accept cmd:<command>", filepath.ToSlash(compiled.PlanPath), openResult.EndLine))
	}

	result := proposeChangeResult{
		TaskID:       taskID,
		PlanPath:     compiled.PlanPath,
		BasePlanHash: basePlanHash,
		Delta: planDeltaDocument{
			SchemaVersion: planDeltaSchemaVersionV01,
			BasePlanHash:  basePlanHash,
			Operations:    operations,
		},
		SuggestedDiff: suggestedDiff,
	}

	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		payload := protocol.Envelope[proposeChangeResult]{
			SchemaVersion: protocol.SchemaVersionV01,
			ToolVersion:   CLIVersion,
			Command:       "propose-change",
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
		fmt.Fprintf(stdout, "base_plan_hash: %s\n", result.BasePlanHash)
		fmt.Fprintf(stdout, "schema_version: %s\n", result.Delta.SchemaVersion)
		fmt.Fprintf(stdout, "operations_count: %d\n", len(result.Delta.Operations))
		for _, op := range result.Delta.Operations {
			fmt.Fprintf(stdout, "- %s %s %s\n", op.OpID, op.Kind, op.Target.TaskID)
		}
		if len(result.SuggestedDiff) > 0 {
			fmt.Fprintln(stdout, "suggested_diff:")
			for _, line := range result.SuggestedDiff {
				fmt.Fprintf(stdout, "- %s\n", line)
			}
		}
	default:
		fmt.Fprintf(stderr, "invalid --format value: %s\n", *format)
		return protocol.ExitUsageError
	}
	return protocol.ExitSuccess
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
