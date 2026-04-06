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
	contextpkg "github.com/vikramoddiraju/planmark/internal/context"
	"github.com/vikramoddiraju/planmark/internal/protocol"
)

func runOpen(args []string, stdout io.Writer, stderr io.Writer) int {
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

	fs := flag.NewFlagSet("open", flag.ContinueOnError)
	fs.SetOutput(stderr)
	planPath := fs.String("plan", "", "path to `plan-file` markdown file")
	format := fs.String("format", "text", "output format: text|json")
	parent := fs.Bool("parent", false, "show parent context (not implemented)")
	neighbors := fs.Bool("neighbors", false, "show neighbor nodes (not implemented)")
	deps := fs.Bool("deps", false, "show dependencies (not implemented)")
	if err := fs.Parse(filteredArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return protocol.ExitSuccess
		}
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitUsageError
	}

	if *parent || *neighbors || *deps {
		fmt.Fprintln(stderr, "--parent/--neighbors/--deps are not implemented yet")
		return protocol.ExitUsageError
	}

	remaining := fs.Args()
	if len(remaining) > 1 {
		fmt.Fprintln(stderr, "usage: plan open <id|node-ref> --plan <path> [--format text|json]")
		return protocol.ExitUsageError
	}
	if len(remaining) == 1 {
		if positionalID != "" {
			fmt.Fprintln(stderr, "too many positional arguments for open")
			return protocol.ExitUsageError
		}
		positionalID = remaining[0]
	}
	if strings.TrimSpace(positionalID) == "" {
		fmt.Fprintln(stderr, "usage: plan open <id|node-ref> --plan <path> [--format text|json]")
		return protocol.ExitUsageError
	}
	if *planPath == "" {
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

	result, err := contextpkg.Open(compiled, positionalID)
	if err != nil {
		if errors.Is(err, contextpkg.ErrTaskNotFound) {
			fmt.Fprintln(stderr, err.Error())
			return protocol.ExitValidationFailed
		}
		fmt.Fprintf(stderr, "open source slice: %v\n", err)
		return protocol.ExitInternalError
	}

	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(result); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return protocol.ExitInternalError
		}
	case "text":
		fmt.Fprintf(stdout, "query_id: %s\n", result.QueryID)
		if result.TaskID != "" {
			fmt.Fprintf(stdout, "task_id: %s\n", result.TaskID)
		}
		fmt.Fprintf(stdout, "node_ref: %s\n", result.NodeRef)
		fmt.Fprintf(stdout, "kind: %s\n", result.Kind)
		fmt.Fprintf(stdout, "title: %s\n", result.Title)
		fmt.Fprintf(stdout, "steps: %d\n", len(result.Steps))
		fmt.Fprintf(stdout, "evidence: %d\n", len(result.Evidence))
		fmt.Fprintf(stdout, "source_path: %s\n", result.SourcePath)
		fmt.Fprintf(stdout, "source_range: %d-%d\n", result.StartLine, result.EndLine)
		fmt.Fprintf(stdout, "slice_hash: %s\n", result.SliceHash)
		fmt.Fprintf(stdout, "slice_text:\n%s\n", result.SliceText)
	default:
		fmt.Fprintf(stderr, "invalid --format value: %s\n", *format)
		return protocol.ExitUsageError
	}

	return protocol.ExitSuccess
}
