package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/compile"
)

func runCompile(args []string, stdout io.Writer, stderr io.Writer) int {
	positionalPlan := ""
	filteredArgs := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--out=") || strings.HasPrefix(arg, "--plan=") || strings.HasPrefix(arg, "--format=") {
			filteredArgs = append(filteredArgs, arg)
			continue
		}
		if arg == "--out" || arg == "--plan" || arg == "--format" {
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
		if positionalPlan == "" {
			positionalPlan = arg
			continue
		}
		filteredArgs = append(filteredArgs, arg)
	}

	fs := flag.NewFlagSet("compile", flag.ContinueOnError)
	fs.SetOutput(stderr)
	planPath := fs.String("plan", "", "path to PLAN markdown file")
	outPath := fs.String("out", "", "path to output plan json")
	format := fs.String("format", "json", "output format: json")
	if err := fs.Parse(filteredArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintln(stderr, err.Error())
		return 2
	}

	remaining := fs.Args()
	if len(remaining) > 1 {
		fmt.Fprintln(stderr, "too many positional arguments for compile")
		return 2
	}
	if len(remaining) == 1 {
		if positionalPlan != "" {
			fmt.Fprintln(stderr, "too many positional arguments for compile")
			return 2
		}
		positionalPlan = remaining[0]
	}
	if positionalPlan != "" {
		if *planPath == "" {
			*planPath = positionalPlan
		} else {
			fmt.Fprintln(stderr, "plan path provided both positionally and via --plan")
			return 2
		}
	}
	if *planPath == "" {
		fmt.Fprintln(stderr, "missing plan path: provide positional path or --plan")
		return 2
	}
	if *format != "json" {
		fmt.Fprintf(stderr, "invalid --format value: %s\n", *format)
		return 2
	}

	data, err := os.ReadFile(*planPath)
	if err != nil {
		fmt.Fprintf(stderr, "read plan: %v\n", err)
		return 1
	}

	compiled, err := compile.CompilePlan(*planPath, data, compile.NewParser(nil))
	if err != nil {
		fmt.Fprintf(stderr, "compile plan: %v\n", err)
		return 1
	}

	payload, err := json.MarshalIndent(compiled, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "encode plan json: %v\n", err)
		return 1
	}
	payload = append(payload, '\n')

	if *outPath != "" {
		if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
			fmt.Fprintf(stderr, "create output dir: %v\n", err)
			return 1
		}
		if err := os.WriteFile(*outPath, payload, 0o644); err != nil {
			fmt.Fprintf(stderr, "write output: %v\n", err)
			return 1
		}
	}

	if _, err := stdout.Write(payload); err != nil {
		fmt.Fprintf(stderr, "write stdout: %v\n", err)
		return 1
	}
	return 0
}
