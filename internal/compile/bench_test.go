package compile

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var (
	benchCompilePlanSink   any
	benchMetadataParseSink MetadataParseResult
	benchAttachSink        MetadataAttachmentResult
)

func BenchmarkCompileMixedPlan(b *testing.B) {
	planPath := filepath.Join("..", "..", "testdata", "plans", "mixed.md")
	content, err := os.ReadFile(planPath)
	if err != nil {
		b.Fatalf("read fixture: %v", err)
	}
	parser := NewParser(nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		compiled, err := CompilePlan(planPath, content, parser)
		if err != nil {
			b.Fatalf("compile plan: %v", err)
		}
		benchCompilePlanSink = compiled
	}
}

func BenchmarkMetadataParseLongLine(b *testing.B) {
	longValue := strings.Repeat("x", 64*1024)
	src := []byte("- [ ] Task A\n  @id bench.task.a\n  @accept cmd:" + longValue + "\n")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parsed, err := ParseMetadata(src)
		if err != nil {
			b.Fatalf("parse metadata: %v", err)
		}
		benchMetadataParseSink = parsed
	}
}

func BenchmarkMetadataAttachManyLines(b *testing.B) {
	const count = 2000
	nodes := make([]Node, 0, count)
	var sb strings.Builder
	for i := 0; i < count; i++ {
		line := i*2 + 1
		nodes = append(nodes, Node{
			Kind:      NodeKindCheckbox,
			Line:      line,
			StartLine: line,
			EndLine:   line,
			Text:      "Task " + strconv.Itoa(i),
		})
		sb.WriteString("- [ ] Task ")
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString("\n")
		sb.WriteString("  @id bench.task.")
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString("\n")
	}
	parsed, err := ParseMetadata([]byte(sb.String()))
	if err != nil {
		b.Fatalf("parse metadata fixture: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		attached := AttachMetadataToNodes(nodes, parsed)
		benchAttachSink = attached
	}
}
