package context

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/compile"
)

func TestL0PacketContainsVerbatimSlice(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task now",
		"  @id fixture.task.now",
		"  @horizon now",
		"  @accept cmd:go test ./...",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	compiled, err := compile.CompilePlan(planPath, []byte(planBody), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	packet, err := BuildL0(compiled, "fixture.task.now")
	if err != nil {
		t.Fatalf("build L0 packet: %v", err)
	}
	if packet.Level != "L0" {
		t.Fatalf("expected level L0, got %q", packet.Level)
	}
	if packet.SourcePath != filepath.ToSlash(planPath) {
		t.Fatalf("expected source path %q, got %q", filepath.ToSlash(planPath), packet.SourcePath)
	}
	if packet.StartLine <= 0 || packet.EndLine < packet.StartLine {
		t.Fatalf("invalid source range: %d-%d", packet.StartLine, packet.EndLine)
	}
	if strings.TrimSpace(packet.SliceHash) == "" {
		t.Fatalf("expected non-empty slice hash")
	}
	if !strings.Contains(packet.SliceText, "Task now") {
		t.Fatalf("expected verbatim task slice, got: %q", packet.SliceText)
	}
}

func TestL0PacketRefusesNowTaskMissingAccept(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task now",
		"  @id fixture.task.now",
		"  @horizon now",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	compiled, err := compile.CompilePlan(planPath, []byte(planBody), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	_, err = BuildL0(compiled, "fixture.task.now")
	if err == nil {
		t.Fatalf("expected readiness error for missing accept")
	}
	if !errors.Is(err, ErrTaskNotReady) {
		t.Fatalf("expected ErrTaskNotReady, got %v", err)
	}
}
