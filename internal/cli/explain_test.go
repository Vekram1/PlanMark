package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/protocol"
)

func TestExplainOutputIncludesAcceptSuggestion(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task now",
		"  @id fixture.task.now",
		"  @horizon now",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := runExplain([]string{"--plan", planPath, "fixture.task.now"}, &out, &errOut)
	if exit != protocol.ExitValidationFailed {
		t.Fatalf("expected validation failure exit, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(out.String(), "suggestion: @accept cmd:<command>") {
		t.Fatalf("expected @accept suggestion in explain output, got: %q", out.String())
	}
}
