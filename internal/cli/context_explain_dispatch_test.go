package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/protocol"
)

func TestRunDispatchesContextAndExplain(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := "- [ ] Task now\n  @id fixture.task.now\n  @horizon now\n  @accept cmd:go test ./...\n"
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"context", "--plan", planPath, "fixture.task.now", "--level", "L0"}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected context exit success, got %d stderr=%q", exit, errOut.String())
	}

	out.Reset()
	errOut.Reset()
	exit = Run([]string{"explain", "--plan", planPath, "fixture.task.now", "--format", "text"}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected explain exit success, got %d stderr=%q", exit, errOut.String())
	}
}
