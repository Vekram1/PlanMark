package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/compile"
)

var (
	benchL0PacketSink any
	benchPathSink     string
)

func BenchmarkContextL0(b *testing.B) {
	planPath := filepath.Join("..", "..", "testdata", "plans", "mixed.md")
	content, err := os.ReadFile(planPath)
	if err != nil {
		b.Fatalf("read fixture: %v", err)
	}
	compiled, err := compile.CompilePlan(planPath, content, compile.NewParser(nil))
	if err != nil {
		b.Fatalf("compile fixture: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		packet, err := BuildL0(compiled, "fixture.mixed.ir")
		if err != nil {
			b.Fatalf("build L0: %v", err)
		}
		benchL0PacketSink = packet
	}
}

func BenchmarkPathNormalizeWorstCase(b *testing.B) {
	repoRoot := b.TempDir()
	worst := strings.Repeat("a/../", 512) + "internal/compile/parser.go"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		normalized, _, err := resolveRepoScopedPath(repoRoot, worst)
		if err != nil {
			b.Fatalf("resolve path: %v", err)
		}
		benchPathSink = normalized
	}
}
