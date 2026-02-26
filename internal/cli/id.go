package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/protocol"
)

var idTokenPattern = regexp.MustCompile(`[^a-z0-9]+`)

type idResult struct {
	Input string `json:"input"`
	ID    string `json:"id"`
}

func runID(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("id", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "text", "output format: text|json")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return protocol.ExitSuccess
		}
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitUsageError
	}
	if len(fs.Args()) != 1 {
		fmt.Fprintln(stderr, "usage: plan id <title> [--format text|json]")
		return protocol.ExitUsageError
	}

	input := strings.TrimSpace(fs.Args()[0])
	id := scaffoldID(input)
	if id == "" {
		fmt.Fprintln(stderr, "unable to derive id from input")
		return protocol.ExitUsageError
	}

	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "text":
		fmt.Fprintln(stdout, id)
		return protocol.ExitSuccess
	case "json":
		payload := protocol.Envelope[idResult]{
			SchemaVersion: protocol.SchemaVersionV01,
			ToolVersion:   CLIVersion,
			Command:       "id",
			Status:        "ok",
			Data: idResult{
				Input: input,
				ID:    id,
			},
		}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(payload); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return protocol.ExitInternalError
		}
		return protocol.ExitSuccess
	default:
		fmt.Fprintf(stderr, "invalid --format value: %s\n", *format)
		return protocol.ExitUsageError
	}
}

func scaffoldID(input string) string {
	value := strings.ToLower(strings.TrimSpace(input))
	value = idTokenPattern.ReplaceAllString(value, ".")
	value = strings.Trim(value, ".")
	return value
}
