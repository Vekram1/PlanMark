package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackCommandExportsPack(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	outPath := filepath.Join(tmp, "pack-out")
	planBody := "- [ ] Task A\n  @id fixture.task.a\n  @horizon now\n  @accept cmd:echo ok\n"
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"pack", "--plan", planPath, "--out", outPath, "--format", "text"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(out.String(), "pack_id:") {
		t.Fatalf("expected text output with pack_id, got %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(outPath, "pack", "index.json")); err != nil {
		t.Fatalf("expected index.json in output pack: %v", err)
	}
}
