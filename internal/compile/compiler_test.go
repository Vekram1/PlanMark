package compile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/ir"
)

func TestSourceRanges(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "testdata", "plans", "parser-order.md")
	fixture, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	compiled, err := CompileNodes(fixturePath, fixture, NewParser(nil))
	if err != nil {
		t.Fatalf("compile nodes: %v", err)
	}

	if len(compiled) == 0 {
		t.Fatalf("expected compiled nodes")
	}

	for _, n := range compiled {
		if n.Slice.StartLine <= 0 || n.Slice.EndLine < n.Slice.StartLine {
			t.Fatalf("invalid source range for node %q: %+v", n.Node.Text, n.Slice)
		}
		if n.Slice.StartLine != n.Node.Line {
			t.Fatalf("expected source range for node %q to start at node line %d, got %d", n.Node.Text, n.Node.Line, n.Slice.StartLine)
		}
		if n.Slice.EndLine < n.Node.Line {
			t.Fatalf("expected source range for node %q to include node line %d, got %d-%d", n.Node.Text, n.Node.Line, n.Slice.StartLine, n.Slice.EndLine)
		}
		if strings.TrimSpace(n.Slice.Text) == "" {
			t.Fatalf("expected non-empty slice text for node %q", n.Node.Text)
		}
	}
}

func TestStructuralNodeScopes(t *testing.T) {
	src := strings.Join([]string{
		"# Root",
		"",
		"## Alpha",
		"- [ ] First checkbox",
		"  @id alpha.first",
		"  child note",
		"- [ ] Second checkbox",
		"## Beta",
		"section prose",
	}, "\n")

	compiled, err := CompileNodes("scopes.md", []byte(src), NewParser(nil))
	if err != nil {
		t.Fatalf("compile nodes: %v", err)
	}

	want := map[string][2]int{
		"Root":            {1, 9},
		"Alpha":           {3, 7},
		"First checkbox":  {4, 6},
		"Second checkbox": {7, 7},
		"Beta":            {8, 9},
	}

	for _, node := range compiled {
		expected, ok := want[node.Node.Text]
		if !ok {
			t.Fatalf("unexpected node in scope test: %q", node.Node.Text)
		}
		if node.Slice.StartLine != expected[0] || node.Slice.EndLine != expected[1] {
			t.Fatalf("unexpected scope for %q: got %d-%d want %d-%d", node.Node.Text, node.Slice.StartLine, node.Slice.EndLine, expected[0], expected[1])
		}
	}
}

func TestSliceHashStability(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "testdata", "plans", "parser-order.md")
	fixture, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	compiledLF, err := CompileNodes(fixturePath, fixture, NewParser(nil))
	if err != nil {
		t.Fatalf("compile LF fixture: %v", err)
	}

	crlf := strings.ReplaceAll(string(fixture), "\n", "\r\n")
	compiledCRLF, err := CompileNodes(fixturePath, []byte(crlf), NewParser(nil))
	if err != nil {
		t.Fatalf("compile CRLF fixture: %v", err)
	}

	if len(compiledLF) != len(compiledCRLF) {
		t.Fatalf("node count mismatch LF=%d CRLF=%d", len(compiledLF), len(compiledCRLF))
	}

	for i := range compiledLF {
		if compiledLF[i].Slice.Hash != compiledCRLF[i].Slice.Hash {
			t.Fatalf("hash mismatch at index %d (%q): LF=%s CRLF=%s", i, compiledLF[i].Node.Text, compiledLF[i].Slice.Hash, compiledCRLF[i].Slice.Hash)
		}
	}
}

func TestMetadataAttachmentRules(t *testing.T) {
	src := strings.Join([]string{
		"@id unattached.pre", // unattached by design (no prior node)
		"# H1",
		"@horizon now",
		"- [ ] task one",
		"  @accept cmd:go test ./...",
		"  @unknown_thing opaque payload",
		"## H2",
		"@deps fixture.h1",
	}, "\n")

	nodes, err := NewParser(nil).Parse("attach.md", []byte(src))
	if err != nil {
		t.Fatalf("parse nodes: %v", err)
	}
	parsed, err := ParseMetadata([]byte(src))
	if err != nil {
		t.Fatalf("parse metadata: %v", err)
	}

	attached := AttachMetadataToNodes(nodes, parsed)
	if len(attached.Nodes) != len(nodes) {
		t.Fatalf("expected %d attached nodes, got %d", len(nodes), len(attached.Nodes))
	}
	if len(attached.Unattached) != 1 {
		t.Fatalf("expected 1 unattached metadata entry, got %d", len(attached.Unattached))
	}
	if attached.Unattached[0].Key != "id" || attached.Unattached[0].Value != "unattached.pre" {
		t.Fatalf("unexpected unattached metadata: %#v", attached.Unattached[0])
	}

	if len(attached.Nodes[0].KnownByKey["horizon"]) != 1 {
		t.Fatalf("expected horizon attached to first node, got %#v", attached.Nodes[0].KnownByKey)
	}
	if got := attached.Nodes[0].KnownByKey["horizon"][0].Value; got != "now" {
		t.Fatalf("unexpected horizon value: %q", got)
	}

	if len(attached.Nodes[1].KnownByKey["accept"]) != 1 {
		t.Fatalf("expected accept attached to second node, got %#v", attached.Nodes[1].KnownByKey)
	}
	if len(attached.Nodes[1].Opaque) != 1 || attached.Nodes[1].Opaque[0].Key != "unknown_thing" {
		t.Fatalf("expected unknown key in opaque metadata for second node, got %#v", attached.Nodes[1].Opaque)
	}

	if len(attached.Nodes[2].KnownByKey["deps"]) != 1 {
		t.Fatalf("expected deps attached to third node, got %#v", attached.Nodes[2].KnownByKey)
	}
}

func TestMetadataAttachmentPrefersEnclosingHeadingForUnindentedSectionMetadata(t *testing.T) {
	src := strings.Join([]string{
		"## Execution",
		"@why section rationale",
		"- [ ] task one",
		"  @accept cmd:go test ./...",
		"@risk section-wide caveat",
		"- [ ] task two",
	}, "\n")

	nodes, err := NewParser(nil).Parse("attach-scope.md", []byte(src))
	if err != nil {
		t.Fatalf("parse nodes: %v", err)
	}
	parsed, err := ParseMetadata([]byte(src))
	if err != nil {
		t.Fatalf("parse metadata: %v", err)
	}

	attached := AttachMetadataToNodes(nodes, parsed)
	if len(attached.Nodes) != len(nodes) {
		t.Fatalf("expected %d attached nodes, got %d", len(nodes), len(attached.Nodes))
	}

	if got := attached.Nodes[0].KnownByKey["why"]; len(got) != 1 || got[0].Value != "section rationale" {
		t.Fatalf("expected heading-level why metadata on section heading, got %#v", attached.Nodes[0].KnownByKey["why"])
	}
	if got := attached.Nodes[0].KnownByKey["risk"]; len(got) != 1 || got[0].Value != "section-wide caveat" {
		t.Fatalf("expected heading-level risk metadata on section heading, got %#v", attached.Nodes[0].KnownByKey["risk"])
	}
	if got := attached.Nodes[1].KnownByKey["accept"]; len(got) != 1 || got[0].Value != "cmd:go test ./..." {
		t.Fatalf("expected indented accept metadata on first checkbox, got %#v", attached.Nodes[1].KnownByKey["accept"])
	}
	if got := attached.Nodes[1].KnownByKey["risk"]; len(got) != 0 {
		t.Fatalf("expected unindented section metadata not to attach to prior checkbox, got %#v", got)
	}
}

func TestCompile(t *testing.T) {
	planPath := filepath.Join("..", "..", "testdata", "plans", "mixed.md")
	content, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("read plan fixture: %v", err)
	}

	compiled, err := CompilePlan(planPath, content, NewParser(nil))
	if err != nil {
		t.Fatalf("CompilePlan failed: %v", err)
	}
	if compiled.IRVersion == "" {
		t.Fatalf("expected ir version")
	}
	if len(compiled.Source.Nodes) == 0 {
		t.Fatalf("expected source nodes")
	}
	if len(compiled.Semantic.Tasks) == 0 {
		t.Fatalf("expected semantic tasks")
	}
	found := false
	for _, task := range compiled.Semantic.Tasks {
		if task.ID == "fixture.mixed.ir" {
			found = true
			if strings.TrimSpace(task.SemanticFingerprint) == "" {
				t.Fatalf("expected semantic fingerprint for task %q", task.ID)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected semantic task id fixture.mixed.ir, got %#v", compiled.Semantic.Tasks)
	}
}

func TestCompilePromotesHeadingTaskWithExplicitTaskMetadata(t *testing.T) {
	src := strings.Join([]string{
		"## Add migration",
		"@id api.migrate",
		"@horizon now",
		"@accept cmd:go test ./...",
		"",
		"We need additive rollout first.",
	}, "\n")

	compiled, err := CompilePlan("heading-task.md", []byte(src), NewParser(nil))
	if err != nil {
		t.Fatalf("compile heading task: %v", err)
	}
	if len(compiled.Semantic.Tasks) != 1 {
		t.Fatalf("expected exactly one semantic task, got %#v", compiled.Semantic.Tasks)
	}
	task := compiled.Semantic.Tasks[0]
	if task.ID != "api.migrate" {
		t.Fatalf("expected heading task id api.migrate, got %#v", task)
	}
	if task.Title != "Add migration" {
		t.Fatalf("expected heading task title Add migration, got %#v", task)
	}
	if task.NodeRef == "" {
		t.Fatalf("expected heading task node_ref")
	}
}

func TestCompileTreatsNestedCheckboxesAsStepsByDefault(t *testing.T) {
	src := strings.Join([]string{
		"## Rollout",
		"- [ ] Parent task",
		"  @id rollout.parent",
		"  @horizon now",
		"  @accept cmd:go test ./...",
		"  - [ ] write migration",
		"  - [x] run verification",
		"- [ ] Sibling task",
		"  @id rollout.sibling",
	}, "\n")

	compiled, err := CompilePlan("nested-steps.md", []byte(src), NewParser(nil))
	if err != nil {
		t.Fatalf("compile nested steps plan: %v", err)
	}
	if len(compiled.Semantic.Tasks) != 2 {
		t.Fatalf("expected 2 semantic tasks, got %#v", compiled.Semantic.Tasks)
	}

	var parent ir.Task
	for _, task := range compiled.Semantic.Tasks {
		if task.ID == "rollout.parent" {
			parent = task
		}
		if strings.Contains(task.Title, "write migration") || strings.Contains(task.Title, "run verification") {
			t.Fatalf("expected nested checkbox not to become standalone task: %#v", task)
		}
	}
	if parent.ID == "" {
		t.Fatalf("expected rollout.parent task, got %#v", compiled.Semantic.Tasks)
	}
	if len(parent.Steps) != 2 {
		t.Fatalf("expected 2 nested steps, got %#v", parent.Steps)
	}
	if parent.Steps[0].Title != "write migration" || parent.Steps[1].Title != "run verification" {
		t.Fatalf("unexpected step titles: %#v", parent.Steps)
	}

	for _, candidate := range compiled.Semantic.TaskCandidates {
		if strings.Contains(candidate.Title, "write migration") || strings.Contains(candidate.Title, "run verification") {
			t.Fatalf("expected nested checkbox not to become standalone task candidate: %#v", candidate)
		}
	}
}

func TestCompileSemanticFingerprintsDeterministic(t *testing.T) {
	planPath := filepath.Join("..", "..", "testdata", "plans", "mixed.md")
	content, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("read plan fixture: %v", err)
	}

	compiledA, err := CompilePlan(planPath, content, NewParser(nil))
	if err != nil {
		t.Fatalf("first compile failed: %v", err)
	}
	compiledB, err := CompilePlan(planPath, content, NewParser(nil))
	if err != nil {
		t.Fatalf("second compile failed: %v", err)
	}
	if len(compiledA.Semantic.Tasks) != len(compiledB.Semantic.Tasks) {
		t.Fatalf("task count mismatch across compiles: %d vs %d", len(compiledA.Semantic.Tasks), len(compiledB.Semantic.Tasks))
	}
	for i := range compiledA.Semantic.Tasks {
		if compiledA.Semantic.Tasks[i].SemanticFingerprint != compiledB.Semantic.Tasks[i].SemanticFingerprint {
			t.Fatalf("fingerprint mismatch for task index %d (%s): %q vs %q",
				i,
				compiledA.Semantic.Tasks[i].ID,
				compiledA.Semantic.Tasks[i].SemanticFingerprint,
				compiledB.Semantic.Tasks[i].SemanticFingerprint,
			)
		}
	}
}

func TestTaskCandidateExtractionDeterministic(t *testing.T) {
	planPath := filepath.Join("..", "..", "testdata", "plans", "mixed.md")
	content, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("read plan fixture: %v", err)
	}

	first, err := CompilePlan(planPath, content, NewParser(nil))
	if err != nil {
		t.Fatalf("first compile failed: %v", err)
	}
	second, err := CompilePlan(planPath, content, NewParser(nil))
	if err != nil {
		t.Fatalf("second compile failed: %v", err)
	}

	if !reflect.DeepEqual(first.Semantic.TaskCandidates, second.Semantic.TaskCandidates) {
		t.Fatalf("task candidate extraction is not deterministic:\nfirst=%#v\nsecond=%#v", first.Semantic.TaskCandidates, second.Semantic.TaskCandidates)
	}
}

func TestTaskCandidateExtractionPreservesProvenance(t *testing.T) {
	planPath := filepath.Join("..", "..", "testdata", "plans", "mixed.md")
	content, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("read plan fixture: %v", err)
	}

	compiled, err := CompilePlan(planPath, content, NewParser(nil))
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	if len(compiled.Semantic.TaskCandidates) == 0 {
		t.Fatalf("expected at least one task candidate")
	}

	for _, candidate := range compiled.Semantic.TaskCandidates {
		if strings.TrimSpace(candidate.NodeRef) == "" {
			t.Fatalf("expected non-empty candidate node_ref: %#v", candidate)
		}
		if filepath.ToSlash(planPath) != candidate.Path {
			t.Fatalf("expected candidate path %q, got %q", filepath.ToSlash(planPath), candidate.Path)
		}
		if candidate.StartLine <= 0 || candidate.EndLine < candidate.StartLine {
			t.Fatalf("invalid candidate line range: %#v", candidate)
		}
		if len(strings.TrimSpace(candidate.SliceHash)) != 64 {
			t.Fatalf("expected sha256 slice hash for candidate: %#v", candidate)
		}
		if strings.TrimSpace(candidate.Title) == "" {
			t.Fatalf("expected candidate title: %#v", candidate)
		}
		if candidate.Kind != string(NodeKindHeading) && candidate.Kind != string(NodeKindCheckbox) {
			t.Fatalf("unexpected candidate kind: %#v", candidate)
		}
	}
}

func TestSourceCoverageNoUnaccountedGaps(t *testing.T) {
	planPath := filepath.Join("..", "..", "testdata", "plans", "mixed.md")
	content, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("read plan fixture: %v", err)
	}

	compiled, err := CompileNodes(planPath, content, NewParser(nil))
	if err != nil {
		t.Fatalf("compile nodes: %v", err)
	}

	coverage, err := ComputeSourceCoverage(content, compiled)
	if err != nil {
		t.Fatalf("compute source coverage: %v", err)
	}
	if coverage.TotalLines <= 0 {
		t.Fatalf("expected positive total lines, got %d", coverage.TotalLines)
	}
	if coverage.Unaccounted != 0 {
		t.Fatalf("expected no unaccounted lines, got %d", coverage.Unaccounted)
	}
	if len(coverage.Interpreted) == 0 {
		t.Fatalf("expected interpreted ranges in coverage")
	}
	if len(coverage.Opaque) == 0 {
		t.Fatalf("expected opaque ranges in mixed fixture coverage")
	}

	accounted := 0
	for _, r := range coverage.Interpreted {
		accounted += r.EndLine - r.StartLine + 1
	}
	for _, r := range coverage.Opaque {
		accounted += r.EndLine - r.StartLine + 1
	}
	if accounted != coverage.TotalLines {
		t.Fatalf("expected total accounted lines to equal total lines (%d), got %d", coverage.TotalLines, accounted)
	}
}

func TestSourceCoverageEmptyContent(t *testing.T) {
	coverage, err := ComputeSourceCoverage(nil, nil)
	if err != nil {
		t.Fatalf("compute source coverage: %v", err)
	}
	if coverage.TotalLines != 0 {
		t.Fatalf("expected zero total lines for empty content, got %d", coverage.TotalLines)
	}
	if coverage.Unaccounted != 0 {
		t.Fatalf("expected zero unaccounted lines for empty content, got %d", coverage.Unaccounted)
	}
	if len(coverage.Interpreted) != 0 || len(coverage.Opaque) != 0 {
		t.Fatalf("expected no coverage ranges for empty content, got interpreted=%v opaque=%v", coverage.Interpreted, coverage.Opaque)
	}
}

func TestLimitsDeterministic(t *testing.T) {
	cases := []struct {
		name       string
		content    string
		limits     Limits
		wantKind   LimitKind
		wantLine   int
		wantActual int
		wantLimit  int
	}{
		{
			name:       "max file bytes",
			content:    strings.Repeat("a", 128),
			limits:     Limits{MaxFileBytes: 64, MaxLineBytes: 1024, MaxNodes: 100, MaxMetadataLinesPerNode: 10},
			wantKind:   LimitKindFileBytes,
			wantLine:   0,
			wantActual: 128,
			wantLimit:  64,
		},
		{
			name:       "max line bytes",
			content:    "# " + strings.Repeat("a", 100) + "\n",
			limits:     Limits{MaxFileBytes: 1024, MaxLineBytes: 16, MaxNodes: 100, MaxMetadataLinesPerNode: 10},
			wantKind:   LimitKindLineBytes,
			wantLine:   1,
			wantActual: 102,
			wantLimit:  16,
		},
		{
			name: "max nodes",
			content: strings.Join([]string{
				"# one",
				"# two",
				"# three",
			}, "\n"),
			limits:     Limits{MaxFileBytes: 1024, MaxLineBytes: 1024, MaxNodes: 2, MaxMetadataLinesPerNode: 10},
			wantKind:   LimitKindNodeCount,
			wantLine:   0,
			wantActual: 3,
			wantLimit:  2,
		},
		{
			name: "max metadata lines per node",
			content: strings.Join([]string{
				"- [ ] task one",
				"@id task.one",
				"@horizon now",
				"@accept cmd:go test ./...",
			}, "\n"),
			limits:     Limits{MaxFileBytes: 1024, MaxLineBytes: 1024, MaxNodes: 100, MaxMetadataLinesPerNode: 2},
			wantKind:   LimitKindMetadataPerNodeLines,
			wantLine:   1,
			wantActual: 3,
			wantLimit:  2,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			parser := NewParser(nil)
			const planPath = "limits.md"

			_, errA := compilePlanWithLimits(planPath, []byte(tc.content), parser, tc.limits)
			_, errB := compilePlanWithLimits(planPath, []byte(tc.content), parser, tc.limits)
			if errA == nil || errB == nil {
				t.Fatalf("expected deterministic limit errors, got errA=%v errB=%v", errA, errB)
			}
			if errA.Error() != errB.Error() {
				t.Fatalf("expected deterministic error text, got A=%q B=%q", errA.Error(), errB.Error())
			}

			var got *LimitError
			if !errors.As(errA, &got) {
				t.Fatalf("expected *LimitError, got %T (%v)", errA, errA)
			}
			if got.Kind != tc.wantKind || got.Line != tc.wantLine || got.Actual != tc.wantActual || got.Limit != tc.wantLimit {
				t.Fatalf("unexpected limit error fields: got=%+v want kind=%s line=%d actual=%d limit=%d",
					got, tc.wantKind, tc.wantLine, tc.wantActual, tc.wantLimit)
			}

			wantText := fmt.Sprintf("compile limit exceeded: kind=%s path=%s", tc.wantKind, planPath)
			if !strings.Contains(got.Error(), wantText) {
				t.Fatalf("expected explicit limit prefix in error text, got %q", got.Error())
			}
		})
	}
}
