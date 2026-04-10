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

	"github.com/vikramoddiraju/planmark/internal/fsio"
	"github.com/vikramoddiraju/planmark/internal/protocol"
)

const (
	stateVersionV01  = "v0.1"
	agentsGuideStart = "<!-- planmark:init:start -->"
	agentsGuideEnd   = "<!-- planmark:init:end -->"
	defaultPlanBody  = `# PLAN

- [ ] Example Task
  @id example.task
  @summary Replace this with your first real task.
`
	defaultConfigBody = `schema_version: v0.1
profiles:
  doctor: loose

# Optional tracker sync defaults:
# tracker:
#   adapter: beads   # or: linear
#   profile: default
`
	defaultAgentsGuideBody = `<!-- planmark:init:start -->
## PlanMark CLI Access (Managed)

When operating in this project, you can use these ` + "`plan`" + ` commands:

- ` + "`plan version --format text|json`" + `
- ` + "`plan init [--dir <path>] [--plan <path>] [--state-dir <path>] [--config <path>] [--no-plan-template] [--no-config] [--format text|json]`" + `
- ` + "`plan compile --plan <path> [--out <path>] [--state-dir <path>]`" + `
- ` + "`plan doctor --plan <path> [--profile loose|build|exec] [--format text|rich|json]`" + `
- ` + "`plan context <id> --plan <path> [--need execute|edit|dependency-check|handoff|auto] [--format text|json]`" + `  ← primary path
- ` + "`plan context <id> --plan <path> --level L0|L1|L2 [--format text|json]`" + `  ← deprecated compatibility path
- ` + "`plan open <id|node-ref> --plan <path> [--format text|json]`" + `
- ` + "`plan explain <id> --plan <path> [--format text|rich|json]`" + `
- ` + "`plan handoff <id|node-ref> --plan <path> [--format text|json]`" + `  ← bounded transfer packet built from need-based selection
- ` + "`plan query --plan <path> [--horizon now|next|later] [--ready|--blocked] [--format text|json]`" + `
- ` + "`plan sync [beads|linear] --plan <path> [--adapter beads|linear] [--profile default|compact|agentic|handoff] [--dry-run] [--format text|json]`" + `
- ` + "`plan changes --plan <path> [--format text|json]`" + `

Canonical truth remains ` + "`PLAN.md`" + `; Beads/tracker state is projected.
<!-- planmark:init:end -->
`
)

type initData struct {
	ProjectDir  string   `json:"project_dir"`
	PlanPath    string   `json:"plan_path"`
	ConfigPath  string   `json:"config_path,omitempty"`
	AgentsPath  string   `json:"agents_path"`
	StateDir    string   `json:"state_dir"`
	Created     []string `json:"created"`
	Updated     []string `json:"updated"`
	Existing    []string `json:"existing"`
	NextCommand string   `json:"next_command"`
}

func runInit(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	projectDir := fs.String("dir", ".", "project directory to initialize")
	planPathFlag := fs.String("plan", "PLAN.md", "plan file path (absolute or relative to --dir)")
	stateDirFlag := fs.String("state-dir", ".planmark", "state directory path (absolute or relative to --dir)")
	configPathFlag := fs.String("config", ".planmark.yaml", "config file path (absolute or relative to --dir)")
	noPlanTemplate := fs.Bool("no-plan-template", false, "do not create PLAN template when plan file is missing")
	noConfig := fs.Bool("no-config", false, "do not create .planmark.yaml when missing")
	format := fs.String("format", "text", "output format: text|json")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return protocol.ExitSuccess
		}
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitUsageError
	}
	if len(fs.Args()) != 0 {
		fmt.Fprintln(stderr, "usage: plan init [--dir <path>] [--plan <path>] [--state-dir <path>] [--config <path>] [--no-plan-template] [--no-config] [--format text|json]")
		return protocol.ExitUsageError
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintf(stderr, "invalid --format value: %s\n", *format)
		return protocol.ExitUsageError
	}

	root, err := filepath.Abs(strings.TrimSpace(*projectDir))
	if err != nil {
		fmt.Fprintf(stderr, "resolve --dir: %v\n", err)
		return protocol.ExitUsageError
	}
	info, err := os.Stat(root)
	if err != nil {
		fmt.Fprintf(stderr, "stat --dir: %v\n", err)
		return protocol.ExitUsageError
	}
	if !info.IsDir() {
		fmt.Fprintf(stderr, "--dir is not a directory: %s\n", root)
		return protocol.ExitUsageError
	}

	resolvePath := func(raw string) string {
		trimmed := strings.TrimSpace(raw)
		if filepath.IsAbs(trimmed) {
			return filepath.Clean(trimmed)
		}
		return filepath.Join(root, trimmed)
	}

	planPath := resolvePath(*planPathFlag)
	stateDir := resolvePath(*stateDirFlag)
	configPath := resolvePath(*configPathFlag)

	created := make([]string, 0, 8)
	updated := make([]string, 0, 4)
	existing := make([]string, 0, 8)

	dirs := []string{
		stateDir,
		filepath.Join(stateDir, "build"),
		filepath.Join(stateDir, "sync"),
		filepath.Join(stateDir, "cache", "context"),
		filepath.Join(stateDir, "cas", "sha256"),
		filepath.Join(stateDir, "journal", "sync"),
		filepath.Join(stateDir, "locks"),
	}
	for _, dir := range dirs {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			existing = append(existing, relToProject(root, dir))
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintf(stderr, "create state dir %s: %v\n", dir, err)
			return protocol.ExitInternalError
		}
		created = append(created, relToProject(root, dir))
	}

	stateVersionPath := filepath.Join(stateDir, "state_version.json")
	if _, err := os.Stat(stateVersionPath); errors.Is(err, os.ErrNotExist) {
		payload := []byte("{\n  \"state_version\": \"" + stateVersionV01 + "\"\n}\n")
		if err := writeFileIfMissing(stateVersionPath, payload, 0o644); err != nil {
			fmt.Fprintf(stderr, "write state version: %v\n", err)
			return protocol.ExitInternalError
		}
		created = append(created, relToProject(root, stateVersionPath))
	} else if err == nil {
		existing = append(existing, relToProject(root, stateVersionPath))
	} else {
		fmt.Fprintf(stderr, "stat state version: %v\n", err)
		return protocol.ExitInternalError
	}

	if _, err := os.Stat(planPath); errors.Is(err, os.ErrNotExist) {
		if !*noPlanTemplate {
			if err := writeFileIfMissing(planPath, []byte(defaultPlanBody), 0o644); err != nil {
				fmt.Fprintf(stderr, "write plan template: %v\n", err)
				return protocol.ExitInternalError
			}
			created = append(created, relToProject(root, planPath))
		}
	} else if err == nil {
		existing = append(existing, relToProject(root, planPath))
	} else {
		fmt.Fprintf(stderr, "stat plan path: %v\n", err)
		return protocol.ExitInternalError
	}

	if !*noConfig {
		if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
			if err := writeFileIfMissing(configPath, []byte(defaultConfigBody), 0o644); err != nil {
				fmt.Fprintf(stderr, "write config: %v\n", err)
				return protocol.ExitInternalError
			}
			created = append(created, relToProject(root, configPath))
		} else if err == nil {
			existing = append(existing, relToProject(root, configPath))
		} else {
			fmt.Fprintf(stderr, "stat config path: %v\n", err)
			return protocol.ExitInternalError
		}
	}
	agentsPath := filepath.Join(root, "AGENTS.md")
	agentsCreated, agentsUpdated, err := ensureAgentsGuide(agentsPath)
	if err != nil {
		fmt.Fprintf(stderr, "write AGENTS.md: %v\n", err)
		return protocol.ExitInternalError
	}
	if agentsCreated {
		created = append(created, relToProject(root, agentsPath))
	} else if agentsUpdated {
		updated = append(updated, relToProject(root, agentsPath))
	} else {
		existing = append(existing, relToProject(root, agentsPath))
	}

	result := initData{
		ProjectDir:  root,
		PlanPath:    planPath,
		ConfigPath:  configPath,
		AgentsPath:  agentsPath,
		StateDir:    stateDir,
		Created:     created,
		Updated:     updated,
		Existing:    existing,
		NextCommand: "plan compile --plan " + planPath + " --out " + filepath.Join(stateDir, "tmp", "plan.json"),
	}
	if *noConfig {
		result.ConfigPath = ""
	}

	switch *format {
	case "json":
		envelope := protocol.Envelope[initData]{
			SchemaVersion: protocol.SchemaVersionV01,
			ToolVersion:   CLIVersion,
			Command:       "init",
			Status:        "ok",
			Data:          result,
		}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(envelope); err != nil {
			fmt.Fprintf(stderr, "write json output: %v\n", err)
			return protocol.ExitInternalError
		}
	default:
		fmt.Fprintf(stdout, "Initialized PlanMark in %s\n", root)
		fmt.Fprintln(stdout, "Created:")
		if len(created) == 0 {
			fmt.Fprintln(stdout, "- (none)")
		} else {
			for _, p := range created {
				fmt.Fprintf(stdout, "- %s\n", p)
			}
		}
		fmt.Fprintln(stdout, "Already existed:")
		if len(existing) == 0 {
			fmt.Fprintln(stdout, "- (none)")
		} else {
			for _, p := range existing {
				fmt.Fprintf(stdout, "- %s\n", p)
			}
		}
		fmt.Fprintln(stdout, "Updated:")
		if len(updated) == 0 {
			fmt.Fprintln(stdout, "- (none)")
		} else {
			for _, p := range updated {
				fmt.Fprintf(stdout, "- %s\n", p)
			}
		}
		fmt.Fprintln(stdout, "Next:")
		fmt.Fprintf(stdout, "- %s\n", result.NextCommand)
	}
	return protocol.ExitSuccess
}

func relToProject(root string, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.Clean(path)
	}
	if rel == "." {
		return rel
	}
	return filepath.Clean(rel)
}

func writeFileIfMissing(path string, payload []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return err
	}
	if _, err := file.Write(payload); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return nil
}

func ensureAgentsGuide(path string) (created bool, updated bool, err error) {
	raw, readErr := os.ReadFile(path)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return false, false, readErr
	}
	if errors.Is(readErr, os.ErrNotExist) {
		if err := fsio.WriteFileAtomic(path, []byte(defaultAgentsGuideBody), 0o644); err != nil {
			return false, false, err
		}
		return true, false, nil
	}

	current := string(raw)
	next := upsertManagedBlock(current, defaultAgentsGuideBody)
	if next == current {
		return false, false, nil
	}
	if err := fsio.WriteFileAtomic(path, []byte(next), 0o644); err != nil {
		return false, false, err
	}
	return false, true, nil
}

func upsertManagedBlock(current string, block string) string {
	start := strings.Index(current, agentsGuideStart)
	end := strings.Index(current, agentsGuideEnd)
	if start >= 0 && end > start {
		end += len(agentsGuideEnd)
		replaced := current[:start] + block + current[end:]
		return strings.TrimSpace(replaced) + "\n"
	}
	trimmed := strings.TrimSpace(current)
	if trimmed == "" {
		return block
	}
	return trimmed + "\n\n" + block
}
