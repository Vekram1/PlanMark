package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/protocol"
)

func FuzzCompileCLIStability(f *testing.F) {
	f.Add([]byte("- [ ] Task A\n  @id task.a\n"), "json", true, false, false)
	f.Add([]byte("- [ ] Task A\n"), "text", false, true, true)

	f.Fuzz(func(t *testing.T, content []byte, format string, positionalPlan bool, duplicatePlan bool, gitDiffHints bool) {
		tmp := t.TempDir()
		planPath := filepath.Join(tmp, "PLAN.md")
		outPath := filepath.Join(tmp, "out", "plan.json")
		if err := os.WriteFile(planPath, content, 0o644); err != nil {
			t.Fatalf("write plan fixture: %v", err)
		}

		args := []string{"compile"}
		if positionalPlan {
			args = append(args, planPath)
		} else {
			args = append(args, "--plan", planPath)
		}
		if duplicatePlan {
			args = append(args, "--plan", planPath)
		}
		args = append(args, "--out", outPath)
		if strings.TrimSpace(format) != "" {
			args = append(args, "--format", format)
		}
		if gitDiffHints {
			args = append(args, "--git-diff-hints")
		}

		exitA, stdoutA, stderrA := runCLI(args)
		exitB, stdoutB, stderrB := runCLI(args)
		if exitA != exitB || stdoutA != stdoutB || stderrA != stderrB {
			t.Fatalf("nondeterministic compile cli result")
		}
		if exitA == protocol.ExitSuccess {
			var payload any
			if err := json.Unmarshal([]byte(stdoutA), &payload); err != nil {
				t.Fatalf("successful compile did not emit valid json: %v", err)
			}
		}
	})
}

func FuzzQueryCLIStability(f *testing.F) {
	f.Add([]byte("- [ ] Task A\n  @id task.a\n  @horizon now\n"), "now", "json", false, false)
	f.Add([]byte("- [ ] Task A\n"), "later", "text", true, false)

	f.Fuzz(func(t *testing.T, content []byte, horizon string, format string, ready bool, blocked bool) {
		tmp := t.TempDir()
		planPath := filepath.Join(tmp, "PLAN.md")
		if err := os.WriteFile(planPath, content, 0o644); err != nil {
			t.Fatalf("write plan fixture: %v", err)
		}

		args := []string{"query", "--plan", planPath}
		if strings.TrimSpace(horizon) != "" {
			args = append(args, "--horizon", horizon)
		}
		if strings.TrimSpace(format) != "" {
			args = append(args, "--format", format)
		}
		if ready {
			args = append(args, "--ready")
		}
		if blocked {
			args = append(args, "--blocked")
		}

		exitA, stdoutA, stderrA := runCLI(args)
		exitB, stdoutB, stderrB := runCLI(args)
		if exitA != exitB || stdoutA != stdoutB || stderrA != stderrB {
			t.Fatalf("nondeterministic query cli result")
		}
		if exitA == protocol.ExitSuccess && strings.EqualFold(strings.TrimSpace(format), "json") {
			var payload any
			if err := json.Unmarshal([]byte(stdoutA), &payload); err != nil {
				t.Fatalf("successful query did not emit valid json: %v", err)
			}
		}
	})
}

func FuzzHasLongFlagDeterminism(f *testing.F) {
	f.Add([]byte("--plan\nPLAN.md"), "plan")
	f.Add([]byte("--format=json"), "format")

	f.Fuzz(func(t *testing.T, encodedArgs []byte, name string) {
		args := splitEncodedArgs(encodedArgs)
		first := hasLongFlag(args, name)
		second := hasLongFlag(append([]string(nil), args...), name)
		if first != second {
			t.Fatalf("nondeterministic long flag detection")
		}
	})
}

func runCLI(args []string) (int, string, string) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := Run(args, &stdout, &stderr)
	return exit, stdout.String(), stderr.String()
}

func splitEncodedArgs(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	parts := strings.Split(string(raw), "\n")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}
