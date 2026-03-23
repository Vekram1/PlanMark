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

func runContext(args []string, stdout io.Writer, stderr io.Writer) int {
	positionalID := ""
	filteredArgs := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--plan=") || strings.HasPrefix(arg, "--level=") || strings.HasPrefix(arg, "--format=") {
			filteredArgs = append(filteredArgs, arg)
			continue
		}
		if arg == "--plan" || arg == "--level" || arg == "--format" {
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

	fs := flag.NewFlagSet("context", flag.ContinueOnError)
	fs.SetOutput(stderr)
	planPath := fs.String("plan", "", "path to PLAN markdown file")
	level := fs.String("level", "L0", "context level: L0|L1|L2")
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
		fmt.Fprintln(stderr, "usage: plan context <id> --plan <path> [--level L0|L1|L2] [--format text|json]")
		return protocol.ExitUsageError
	}
	if len(remaining) == 1 {
		if positionalID != "" {
			fmt.Fprintln(stderr, "too many positional arguments for context")
			return protocol.ExitUsageError
		}
		positionalID = remaining[0]
	}
	if strings.TrimSpace(positionalID) == "" {
		fmt.Fprintln(stderr, "usage: plan context <id> --plan <path> [--level L0|L1|L2] [--format text|json]")
		return protocol.ExitUsageError
	}
	if *planPath == "" {
		fmt.Fprintln(stderr, "missing --plan")
		return protocol.ExitUsageError
	}

	taskID := strings.TrimSpace(positionalID)
	if taskID == "" {
		fmt.Fprintln(stderr, "missing task id")
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

	levelNormalized := strings.ToUpper(strings.TrimSpace(*level))

	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		switch levelNormalized {
		case "L0":
			packet, err := contextpkg.BuildL0(compiled, taskID)
			if err != nil {
				if errors.Is(err, contextpkg.ErrTaskNotReady) || errors.Is(err, contextpkg.ErrTaskNotFound) {
					fmt.Fprintln(stderr, err.Error())
					return protocol.ExitValidationFailed
				}
				fmt.Fprintf(stderr, "build context: %v\n", err)
				return protocol.ExitInternalError
			}
			payload := protocol.Envelope[contextpkg.L0Packet]{
				SchemaVersion: protocol.SchemaVersionV01,
				ToolVersion:   CLIVersion,
				Command:       "context",
				Status:        "ok",
				Data:          packet,
			}
			enc := json.NewEncoder(stdout)
			enc.SetEscapeHTML(false)
			if err := enc.Encode(payload); err != nil {
				fmt.Fprintln(stderr, err.Error())
				return protocol.ExitInternalError
			}
		case "L1":
			packet, err := contextpkg.BuildL1(compiled, taskID)
			if err != nil {
				if errors.Is(err, contextpkg.ErrTaskNotReady) || errors.Is(err, contextpkg.ErrTaskNotFound) {
					fmt.Fprintln(stderr, err.Error())
					return protocol.ExitValidationFailed
				}
				fmt.Fprintf(stderr, "build context: %v\n", err)
				return protocol.ExitInternalError
			}
			payload := protocol.Envelope[contextpkg.L1Packet]{
				SchemaVersion: protocol.SchemaVersionV01,
				ToolVersion:   CLIVersion,
				Command:       "context",
				Status:        "ok",
				Data:          packet,
			}
			enc := json.NewEncoder(stdout)
			enc.SetEscapeHTML(false)
			if err := enc.Encode(payload); err != nil {
				fmt.Fprintln(stderr, err.Error())
				return protocol.ExitInternalError
			}
		case "L2":
			packet, err := contextpkg.BuildL2(compiled, taskID)
			if err != nil {
				if errors.Is(err, contextpkg.ErrTaskNotReady) || errors.Is(err, contextpkg.ErrTaskNotFound) {
					fmt.Fprintln(stderr, err.Error())
					return protocol.ExitValidationFailed
				}
				fmt.Fprintf(stderr, "build context: %v\n", err)
				return protocol.ExitInternalError
			}
			payload := protocol.Envelope[contextpkg.L2Packet]{
				SchemaVersion: protocol.SchemaVersionV01,
				ToolVersion:   CLIVersion,
				Command:       "context",
				Status:        "ok",
				Data:          packet,
			}
			enc := json.NewEncoder(stdout)
			enc.SetEscapeHTML(false)
			if err := enc.Encode(payload); err != nil {
				fmt.Fprintln(stderr, err.Error())
				return protocol.ExitInternalError
			}
		default:
			fmt.Fprintf(stderr, "level %q not implemented yet (supported: L0|L1|L2)\n", *level)
			return protocol.ExitUsageError
		}
	case "text":
		switch levelNormalized {
		case "L0":
			packet, err := contextpkg.BuildL0(compiled, taskID)
			if err != nil {
				if errors.Is(err, contextpkg.ErrTaskNotReady) || errors.Is(err, contextpkg.ErrTaskNotFound) {
					fmt.Fprintln(stderr, err.Error())
					return protocol.ExitValidationFailed
				}
				fmt.Fprintf(stderr, "build context: %v\n", err)
				return protocol.ExitInternalError
			}
			fmt.Fprintf(stdout, "level: %s\n", packet.Level)
			fmt.Fprintf(stdout, "task_id: %s\n", packet.TaskID)
			fmt.Fprintf(stdout, "title: %s\n", packet.Title)
			fmt.Fprintf(stdout, "horizon: %s\n", packet.Horizon)
			fmt.Fprintf(stdout, "steps: %d\n", len(packet.Steps))
			fmt.Fprintf(stdout, "evidence: %d\n", len(packet.Evidence))
			fmt.Fprintf(stdout, "source_path: %s\n", packet.SourcePath)
			fmt.Fprintf(stdout, "source_range: %d-%d\n", packet.StartLine, packet.EndLine)
			fmt.Fprintf(stdout, "slice_hash: %s\n", packet.SliceHash)
			fmt.Fprintf(stdout, "slice_text:\n%s\n", packet.SliceText)
		case "L1":
			packet, err := contextpkg.BuildL1(compiled, taskID)
			if err != nil {
				if errors.Is(err, contextpkg.ErrTaskNotReady) || errors.Is(err, contextpkg.ErrTaskNotFound) {
					fmt.Fprintln(stderr, err.Error())
					return protocol.ExitValidationFailed
				}
				fmt.Fprintf(stderr, "build context: %v\n", err)
				return protocol.ExitInternalError
			}
			fmt.Fprintf(stdout, "level: %s\n", packet.Level)
			fmt.Fprintf(stdout, "task_id: %s\n", packet.TaskID)
			fmt.Fprintf(stdout, "title: %s\n", packet.Title)
			fmt.Fprintf(stdout, "horizon: %s\n", packet.Horizon)
			fmt.Fprintf(stdout, "steps: %d\n", len(packet.Steps))
			fmt.Fprintf(stdout, "evidence: %d\n", len(packet.Evidence))
			fmt.Fprintf(stdout, "source_path: %s\n", packet.SourcePath)
			fmt.Fprintf(stdout, "source_range: %d-%d\n", packet.StartLine, packet.EndLine)
			fmt.Fprintf(stdout, "slice_hash: %s\n", packet.SliceHash)
			fmt.Fprintf(stdout, "pins: %d\n", len(packet.Pins))
		case "L2":
			packet, err := contextpkg.BuildL2(compiled, taskID)
			if err != nil {
				if errors.Is(err, contextpkg.ErrTaskNotReady) || errors.Is(err, contextpkg.ErrTaskNotFound) {
					fmt.Fprintln(stderr, err.Error())
					return protocol.ExitValidationFailed
				}
				fmt.Fprintf(stderr, "build context: %v\n", err)
				return protocol.ExitInternalError
			}
			fmt.Fprintf(stdout, "level: %s\n", packet.Level)
			fmt.Fprintf(stdout, "task_id: %s\n", packet.TaskID)
			fmt.Fprintf(stdout, "title: %s\n", packet.Title)
			fmt.Fprintf(stdout, "horizon: %s\n", packet.Horizon)
			fmt.Fprintf(stdout, "steps: %d\n", len(packet.Steps))
			fmt.Fprintf(stdout, "evidence: %d\n", len(packet.Evidence))
			fmt.Fprintf(stdout, "source_path: %s\n", packet.SourcePath)
			fmt.Fprintf(stdout, "source_range: %d-%d\n", packet.StartLine, packet.EndLine)
			fmt.Fprintf(stdout, "slice_hash: %s\n", packet.SliceHash)
			fmt.Fprintf(stdout, "closure: %d\n", len(packet.Closure))
		default:
			fmt.Fprintf(stderr, "level %q not implemented yet (supported: L0|L1|L2)\n", *level)
			return protocol.ExitUsageError
		}
	default:
		fmt.Fprintf(stderr, "invalid --format value: %s\n", *format)
		return protocol.ExitUsageError
	}

	return protocol.ExitSuccess
}
