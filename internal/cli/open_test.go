package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/compile"
	"github.com/vikramoddiraju/planmark/internal/protocol"
)

func TestOpenSupportsNodeRefForNonTaskSlice(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"# Overview",
		"- [ ] Task now",
		"  @id fixture.task.now",
		"  @horizon now",
		"  @accept cmd:go test ./...",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	compiled, err := compile.CompilePlan(planPath, []byte(planBody), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	var headingRef string
	for _, node := range compiled.Source.Nodes {
		if node.Kind == "heading" {
			headingRef = node.NodeRef
			break
		}
	}
	if headingRef == "" {
		t.Fatalf("expected heading node_ref")
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := runOpen([]string{"--plan", planPath, headingRef}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected success, got %d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(out.String(), "node_ref: "+headingRef) {
		t.Fatalf("expected output to include heading node_ref, got %q", out.String())
	}
	if !strings.Contains(out.String(), "slice_text:\n# Overview") {
		t.Fatalf("expected exact heading slice text, got %q", out.String())
	}
}
