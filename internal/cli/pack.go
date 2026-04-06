package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/pack"
	"github.com/vikramoddiraju/planmark/internal/protocol"
)

type packOutput struct {
	PackID    string `json:"pack_id"`
	Output    string `json:"output"`
	IndexPath string `json:"index_path"`
	TaskCount int    `json:"task_count"`
}

func runPack(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("pack", flag.ContinueOnError)
	fs.SetOutput(stderr)
	planPath := fs.String("plan", "", "path to `plan-file` markdown file")
	idsArg := fs.String("ids", "", "comma-separated task ids")
	horizon := fs.String("horizon", "", "task horizon filter: now|next")
	levelArg := fs.String("level", "L0", "context level(s): L0|L1|L2 or comma-separated")
	outPath := fs.String("out", "", "write pack to `output-path` (directory or .tar.gz)")
	format := fs.String("format", "json", "output format: text|json")
	stateDir := fs.String("state-dir", "", "optional state directory")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return protocol.ExitSuccess
		}
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitUsageError
	}
	if strings.TrimSpace(*planPath) == "" {
		fmt.Fprintln(stderr, "missing --plan")
		return protocol.ExitUsageError
	}
	if strings.TrimSpace(*outPath) == "" {
		fmt.Fprintln(stderr, "missing --out")
		return protocol.ExitUsageError
	}

	ids := splitNonEmpty(*idsArg)
	levels := splitNonEmpty(*levelArg)
	result, err := pack.Export(pack.Options{
		PlanPath: *planPath,
		StateDir: *stateDir,
		IDs:      ids,
		Horizon:  *horizon,
		Levels:   levels,
		OutPath:  *outPath,
	})
	if err != nil {
		fmt.Fprintf(stderr, "pack export: %v\n", err)
		return protocol.ExitValidationFailed
	}

	response := packOutput{
		PackID:    result.PackID,
		Output:    result.Output,
		IndexPath: result.IndexPath,
		TaskCount: result.TaskCount,
	}
	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		payload := protocol.Envelope[packOutput]{
			SchemaVersion: protocol.SchemaVersionV01,
			ToolVersion:   CLIVersion,
			Command:       "pack",
			Status:        "ok",
			Data:          response,
		}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(payload); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return protocol.ExitInternalError
		}
	case "text":
		fmt.Fprintf(stdout, "pack_id: %s\n", result.PackID)
		fmt.Fprintf(stdout, "output: %s\n", result.Output)
		fmt.Fprintf(stdout, "index_path: %s\n", result.IndexPath)
		fmt.Fprintf(stdout, "task_count: %d\n", result.TaskCount)
	default:
		fmt.Fprintf(stderr, "invalid --format value: %s\n", *format)
		return protocol.ExitUsageError
	}

	return protocol.ExitSuccess
}

func splitNonEmpty(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}
