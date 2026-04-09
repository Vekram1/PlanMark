package context

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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

func TestL0PacketSupportsPromotedHeadingTasks(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"## Add migration",
		"@id api.migrate",
		"@horizon now",
		"@accept cmd:go test ./...",
		"",
		"Additive rollout only.",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	compiled, err := compile.CompilePlan(planPath, []byte(planBody), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	packet, err := BuildL0(compiled, "api.migrate")
	if err != nil {
		t.Fatalf("build heading L0 packet: %v", err)
	}
	if packet.TaskID != "api.migrate" {
		t.Fatalf("expected heading task id api.migrate, got %#v", packet)
	}
	if !strings.Contains(packet.SliceText, "## Add migration") || !strings.Contains(packet.SliceText, "Additive rollout only.") {
		t.Fatalf("expected heading scope slice text, got %q", packet.SliceText)
	}
}

func TestL0PacketIncludesStepsAndEvidence(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"## Add migration",
		"@id api.migrate",
		"@horizon now",
		"@accept cmd:go test ./...",
		"",
		"- [ ] write migration",
		"- [x] run verification",
		"### Evidence",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	compiled, err := compile.CompilePlan(planPath, []byte(planBody), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	packet, err := BuildL0(compiled, "api.migrate")
	if err != nil {
		t.Fatalf("build L0 packet: %v", err)
	}
	if len(packet.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %#v", packet.Steps)
	}
	if packet.Steps[0].Title != "write migration" || packet.Steps[1].Title != "run verification" {
		t.Fatalf("unexpected step titles: %#v", packet.Steps)
	}
	if len(packet.Evidence) != 1 {
		t.Fatalf("expected 1 evidence block, got %#v", packet.Evidence)
	}
	if packet.Evidence[0].Kind != "heading" || !strings.Contains(packet.Evidence[0].SliceText, "### Evidence") {
		t.Fatalf("unexpected evidence block: %#v", packet.Evidence[0])
	}
}

func TestL0PacketIncludesSemanticSections(t *testing.T) {
	content := strings.Join([]string{
		"## Add migration",
		"@id api.migrate",
		"@horizon now",
		"@accept cmd:go test ./...",
		"",
		"We need additive rollout first.",
	}, "\n")

	compiled, err := compile.CompilePlan("PLAN.md", []byte(content), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	packet, err := BuildL0(compiled, "api.migrate")
	if err != nil {
		t.Fatalf("build L0 packet: %v", err)
	}
	if len(packet.Sections) != 1 {
		t.Fatalf("expected one semantic section, got %#v", packet.Sections)
	}
	if packet.Sections[0].Title != "Details" {
		t.Fatalf("expected details section title, got %#v", packet.Sections)
	}
	if len(packet.Sections[0].Body) != 1 || packet.Sections[0].Body[0] != "We need additive rollout first." {
		t.Fatalf("expected section body in L0 packet, got %#v", packet.Sections)
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

func TestL1IncludesPinExtracts(t *testing.T) {
	tmp := t.TempDir()
	targetPath := filepath.Join(tmp, "internal", "compile", "parser.go")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("mkdir target path: %v", err)
	}
	targetBody := strings.Join([]string{
		"package compile",
		"",
		"func Parse() {}",
	}, "\n")
	if err := os.WriteFile(targetPath, []byte(targetBody), 0o644); err != nil {
		t.Fatalf("write target file: %v", err)
	}

	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task now",
		"  @id fixture.task.now",
		"  @horizon now",
		"  @accept cmd:go test ./...",
		"  @touches internal/compile/parser.go:1-2",
		"  @pin internal/compile/parser.go:3",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	compiled, err := compile.CompilePlan(planPath, []byte(planBody), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	packet, err := BuildL1(compiled, "fixture.task.now")
	if err != nil {
		t.Fatalf("build L1 packet: %v", err)
	}
	if packet.Level != "L1" {
		t.Fatalf("expected level L1, got %q", packet.Level)
	}
	if len(packet.Pins) != 2 {
		t.Fatalf("expected 2 pin extracts, got %d", len(packet.Pins))
	}

	first := packet.Pins[0]
	if first.Key != "touches" {
		t.Fatalf("expected first key touches, got %q", first.Key)
	}
	if first.TargetPath != "internal/compile/parser.go" {
		t.Fatalf("expected target path internal/compile/parser.go, got %q", first.TargetPath)
	}
	if first.StartLine != 1 || first.EndLine != 2 {
		t.Fatalf("expected line range 1-2, got %d-%d", first.StartLine, first.EndLine)
	}
	expectedFirstSlice := "package compile\n"
	expectedFirstHash := sha256.Sum256([]byte(expectedFirstSlice))
	if first.TargetHash != hex.EncodeToString(expectedFirstHash[:]) {
		t.Fatalf("unexpected first target hash: %q", first.TargetHash)
	}

	second := packet.Pins[1]
	if second.Key != "pin" {
		t.Fatalf("expected second key pin, got %q", second.Key)
	}
	if second.StartLine != 3 || second.EndLine != 3 {
		t.Fatalf("expected line range 3-3, got %d-%d", second.StartLine, second.EndLine)
	}
	if strings.TrimSpace(second.SliceText) != "func Parse() {}" {
		t.Fatalf("expected pin slice text for line 3, got %q", second.SliceText)
	}
	if second.Freshness != "fresh" {
		t.Fatalf("expected fresh pin status, got %q", second.Freshness)
	}
}

func TestL1PinFreshnessDetectsTargetHashChange(t *testing.T) {
	tmp := t.TempDir()
	stateDir := filepath.Join(tmp, ".planmark")
	targetPath := filepath.Join(tmp, "internal", "compile", "parser.go")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("mkdir target path: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("package compile\n"), 0o644); err != nil {
		t.Fatalf("write target file: %v", err)
	}

	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task now",
		"  @id fixture.task.now",
		"  @horizon now",
		"  @accept cmd:go test ./...",
		"  @pin internal/compile/parser.go",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	compiled, err := compile.CompilePlan(planPath, []byte(planBody), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	packetA, _, err := BuildL1Cached(compiled, "fixture.task.now", stateDir)
	if err != nil {
		t.Fatalf("build initial L1 cached: %v", err)
	}
	if len(packetA.Pins) != 1 {
		t.Fatalf("expected 1 pin, got %d", len(packetA.Pins))
	}
	initialHash := packetA.Pins[0].TargetHash
	if packetA.Pins[0].Freshness != "fresh" {
		t.Fatalf("expected initial freshness=fresh, got %q", packetA.Pins[0].Freshness)
	}

	if err := os.WriteFile(targetPath, []byte("package compile\n// changed\n"), 0o644); err != nil {
		t.Fatalf("update target file: %v", err)
	}

	packetB, _, err := BuildL1Cached(compiled, "fixture.task.now", stateDir)
	if err != nil {
		t.Fatalf("build stale L1 cached: %v", err)
	}
	if len(packetB.Pins) != 1 {
		t.Fatalf("expected 1 pin, got %d", len(packetB.Pins))
	}
	pin := packetB.Pins[0]
	if pin.Freshness != "stale" {
		t.Fatalf("expected stale freshness, got %q", pin.Freshness)
	}
	if pin.Baseline != initialHash {
		t.Fatalf("expected baseline hash %q, got %q", initialHash, pin.Baseline)
	}

	raw, err := json.Marshal(packetB)
	if err != nil {
		t.Fatalf("marshal stale packet: %v", err)
	}
	if !strings.Contains(string(raw), "\"freshness\":\"stale\"") {
		t.Fatalf("expected JSON freshness status, got %s", string(raw))
	}
}

func TestPinFreshnessIdentityDistinguishesRangeForms(t *testing.T) {
	a := PinExtract{Key: "pin", TargetPath: "internal/compile/parser.go", StartLine: 1, identityEnd: 0}
	b := PinExtract{Key: "pin", TargetPath: "internal/compile/parser.go", StartLine: 1, identityEnd: 1}
	if pinIdentity(a) == pinIdentity(b) {
		t.Fatalf("expected distinct pin freshness identities for implicit vs explicit ranges")
	}
}

func TestPinPathRepoScopeEnforced(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task now",
		"  @id fixture.task.now",
		"  @horizon now",
		"  @accept cmd:go test ./...",
		"  @pin ../outside.txt:1",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	compiled, err := compile.CompilePlan(planPath, []byte(planBody), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	_, err = BuildL1(compiled, "fixture.task.now")
	if err == nil {
		t.Fatalf("expected repo-scope path validation error")
	}
	if !strings.Contains(err.Error(), "escapes repository root") {
		t.Fatalf("expected repository root escape error, got %v", err)
	}
}

func TestPinPathSymlinkPolicy(t *testing.T) {
	tmp := t.TempDir()
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	linkPath := filepath.Join(tmp, "linked.txt")
	if err := os.Symlink(outsideFile, linkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task now",
		"  @id fixture.task.now",
		"  @horizon now",
		"  @accept cmd:go test ./...",
		"  @pin linked.txt:1",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	compiled, err := compile.CompilePlan(planPath, []byte(planBody), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	_, err = BuildL1(compiled, "fixture.task.now")
	if err == nil {
		t.Fatalf("expected symlink policy error")
	}
	if !strings.Contains(err.Error(), "resolves outside repository root via symlink") {
		t.Fatalf("expected symlink escape error, got %v", err)
	}
}

func TestL1PinWithoutExplicitRangeUsesWholeFile(t *testing.T) {
	tmp := t.TempDir()
	targetPath := filepath.Join(tmp, "internal", "compile", "parser.go")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("mkdir target path: %v", err)
	}
	targetBody := strings.Join([]string{
		"package compile",
		"",
		"func Parse() {}",
	}, "\n")
	if err := os.WriteFile(targetPath, []byte(targetBody), 0o644); err != nil {
		t.Fatalf("write target file: %v", err)
	}

	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task now",
		"  @id fixture.task.now",
		"  @horizon now",
		"  @accept cmd:go test ./...",
		"  @touches internal/compile/parser.go",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	compiled, err := compile.CompilePlan(planPath, []byte(planBody), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	packet, err := BuildL1(compiled, "fixture.task.now")
	if err != nil {
		t.Fatalf("build L1 packet: %v", err)
	}
	if len(packet.Pins) != 1 {
		t.Fatalf("expected 1 pin extract, got %d", len(packet.Pins))
	}
	if packet.Pins[0].StartLine != 1 || packet.Pins[0].EndLine != 3 {
		t.Fatalf("expected whole-file line range 1-3, got %d-%d", packet.Pins[0].StartLine, packet.Pins[0].EndLine)
	}
	if packet.Pins[0].SliceText != targetBody {
		t.Fatalf("expected whole-file slice text, got %q", packet.Pins[0].SliceText)
	}
}

func TestL2IncludesDependencyClosureSummaries(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Root task",
		"  @id fixture.task.root",
		"  @horizon later",
		"  @deps fixture.task.b,fixture.task.c",
		"",
		"- [ ] Dependency B",
		"  @id fixture.task.b",
		"  @horizon later",
		"  @deps fixture.task.c",
		"",
		"- [ ] Dependency C",
		"  @id fixture.task.c",
		"  @horizon later",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	compiled, err := compile.CompilePlan(planPath, []byte(planBody), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	packet, err := BuildL2(compiled, "fixture.task.root")
	if err != nil {
		t.Fatalf("build L2 packet: %v", err)
	}
	if packet.Level != "L2" {
		t.Fatalf("expected level L2, got %q", packet.Level)
	}
	if len(packet.Closure) != 2 {
		t.Fatalf("expected 2 closure dependencies, got %d", len(packet.Closure))
	}
	if packet.Closure[0].TaskID != "fixture.task.b" || packet.Closure[1].TaskID != "fixture.task.c" {
		t.Fatalf("unexpected deterministic closure order: %+v", packet.Closure)
	}
	for _, dep := range packet.Closure {
		if dep.SourcePath != filepath.ToSlash(planPath) {
			t.Fatalf("expected source path %q, got %q", filepath.ToSlash(planPath), dep.SourcePath)
		}
		if dep.StartLine <= 0 || dep.EndLine < dep.StartLine {
			t.Fatalf("invalid source range for %s: %d-%d", dep.TaskID, dep.StartLine, dep.EndLine)
		}
		if strings.TrimSpace(dep.SliceHash) == "" {
			t.Fatalf("missing slice hash for %s", dep.TaskID)
		}
	}
}

func TestL2FailsOnUnresolvedDependency(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Root task",
		"  @id fixture.task.root",
		"  @horizon later",
		"  @deps fixture.task.missing",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	compiled, err := compile.CompilePlan(planPath, []byte(planBody), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	_, err = BuildL2(compiled, "fixture.task.root")
	if err == nil {
		t.Fatalf("expected unresolved dependency error")
	}
	if !strings.Contains(err.Error(), "dependency task not found") {
		t.Fatalf("expected unresolved dependency error, got %v", err)
	}
}

func TestL1PinsIgnoreMetadataInsideFencedCode(t *testing.T) {
	tmp := t.TempDir()
	targetPath := filepath.Join(tmp, "internal", "compile", "parser.go")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("mkdir target path: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("package compile\n"), 0o644); err != nil {
		t.Fatalf("write target file: %v", err)
	}

	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task now",
		"  @id fixture.task.now",
		"  @horizon now",
		"  @accept cmd:go test ./...",
		"  ```md",
		"  @pin internal/compile/parser.go:1",
		"  ```",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	compiled, err := compile.CompilePlan(planPath, []byte(planBody), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	packet, err := BuildL1(compiled, "fixture.task.now")
	if err != nil {
		t.Fatalf("build L1 packet: %v", err)
	}
	if len(packet.Pins) != 0 {
		t.Fatalf("expected no pins extracted from fenced code, got %d", len(packet.Pins))
	}
}

func TestL1PinRejectsReversedRange(t *testing.T) {
	tmp := t.TempDir()
	targetPath := filepath.Join(tmp, "internal", "compile", "parser.go")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("mkdir target path: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("package compile\n"), 0o644); err != nil {
		t.Fatalf("write target file: %v", err)
	}

	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task now",
		"  @id fixture.task.now",
		"  @horizon now",
		"  @accept cmd:go test ./...",
		"  @pin internal/compile/parser.go:3-1",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	compiled, err := compile.CompilePlan(planPath, []byte(planBody), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	_, err = BuildL1(compiled, "fixture.task.now")
	if err == nil {
		t.Fatalf("expected invalid range error")
	}
	if !strings.Contains(err.Error(), "invalid range") {
		t.Fatalf("expected invalid range error, got %v", err)
	}
}

func TestSelectByNeedEditRejectsBrokenTouches(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task now",
		"  @id fixture.task.now",
		"  @horizon now",
		"  @accept cmd:go test ./...",
		"  @touches internal/compile/parser.go:9-3",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	compiled, err := compile.CompilePlan(planPath, []byte(planBody), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	_, err = SelectByNeed(compiled, "fixture.task.now", NeedEdit)
	if err == nil {
		t.Fatalf("expected need-based selector to reject broken touches metadata")
	}
	if !strings.Contains(err.Error(), "invalid range") {
		t.Fatalf("expected invalid range error, got %v", err)
	}
}

func TestSelectByNeedEditUsesAcceptanceRepoFileReference(t *testing.T) {
	tmp := t.TempDir()
	targetPath := filepath.Join(tmp, "docs", "specs", "context-selection-v0.1.md")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("mkdir target path: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("# spec\n"), 0o644); err != nil {
		t.Fatalf("write target file: %v", err)
	}

	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task now",
		"  @id fixture.task.now",
		"  @horizon now",
		"  @accept cmd:test -f docs/specs/context-selection-v0.1.md",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	compiled, err := compile.CompilePlan(planPath, []byte(planBody), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	packet, err := SelectByNeed(compiled, "fixture.task.now", NeedEdit)
	if err != nil {
		t.Fatalf("select by need: %v", err)
	}
	if packet.SelectedContextClass != "task+files" {
		t.Fatalf("expected task+files, got %#v", packet)
	}
	if len(packet.IncludedFiles) != 1 || packet.IncludedFiles[0] != "docs/specs/context-selection-v0.1.md" {
		t.Fatalf("expected inferred included file, got %#v", packet.IncludedFiles)
	}
	if len(packet.IncludedFileRefs) != 1 || packet.IncludedFileRefs[0].Path != "docs/specs/context-selection-v0.1.md" || packet.IncludedFileRefs[0].Reason != "acceptance or scoped task text references repo files" {
		t.Fatalf("expected structured included file refs, got %#v", packet.IncludedFileRefs)
	}
	if len(packet.Pins) != 1 || packet.Pins[0].Key != "inferred" {
		t.Fatalf("expected inferred file pin, got %#v", packet.Pins)
	}
	if len(packet.EscalationReasons) != 1 || packet.EscalationReasons[0] != "acceptance references repo files" {
		t.Fatalf("expected acceptance-based escalation reason, got %#v", packet.EscalationReasons)
	}
}

func TestSelectByNeedAutoUsesScopedTaskRepoFileReference(t *testing.T) {
	tmp := t.TempDir()
	targetPath := filepath.Join(tmp, "internal", "context", "selector.go")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("mkdir target path: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("package context\n"), 0o644); err != nil {
		t.Fatalf("write target file: %v", err)
	}

	docPath := filepath.Join(tmp, "docs", "specs", "context-selection-v0.1.md")
	if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
		t.Fatalf("mkdir doc path: %v", err)
	}
	if err := os.WriteFile(docPath, []byte("# spec\n"), 0o644); err != nil {
		t.Fatalf("write doc file: %v", err)
	}

	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"## Implement selector",
		"@id pm.context.impl",
		"@horizon now",
		"@accept cmd:test -f docs/specs/context-selection-v0.1.md",
		"",
		"Update internal/context/selector.go to add deterministic path-sensitive selection.",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	compiled, err := compile.CompilePlan(planPath, []byte(planBody), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	packet, err := SelectByNeed(compiled, "pm.context.impl", NeedAuto)
	if err != nil {
		t.Fatalf("select by need: %v", err)
	}
	if packet.SelectedContextClass != "task+files" {
		t.Fatalf("expected task+files, got %#v", packet)
	}
	if got := packet.IncludedFiles; len(got) != 2 || got[0] != "docs/specs/context-selection-v0.1.md" || got[1] != "internal/context/selector.go" {
		t.Fatalf("expected sorted inferred file list, got %#v", got)
	}
	if got := packet.IncludedFileRefs; len(got) != 2 || got[0].Path != "docs/specs/context-selection-v0.1.md" || got[0].Reason != "acceptance or scoped task text references repo files" || got[1].Path != "internal/context/selector.go" || got[1].Reason != "acceptance or scoped task text references repo files" {
		t.Fatalf("expected structured included file refs, got %#v", got)
	}
	if len(packet.EscalationReasons) != 2 {
		t.Fatalf("expected two escalation reasons, got %#v", packet.EscalationReasons)
	}
	if packet.EscalationReasons[0] != "acceptance references repo files" || packet.EscalationReasons[1] != "scoped task text references repo files" {
		t.Fatalf("unexpected escalation reasons: %#v", packet.EscalationReasons)
	}
}

func TestSelectByNeedAutoIgnoresRepoPathsInsideFencedSectionExamples(t *testing.T) {
	tmp := t.TempDir()
	docPath := filepath.Join(tmp, "docs", "specs", "context-selection-v0.1.md")
	if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
		t.Fatalf("mkdir doc path: %v", err)
	}
	if err := os.WriteFile(docPath, []byte("# spec\n"), 0o644); err != nil {
		t.Fatalf("write doc file: %v", err)
	}

	targetPath := filepath.Join(tmp, "internal", "context", "selector.go")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("mkdir target path: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("package context\n"), 0o644); err != nil {
		t.Fatalf("write target file: %v", err)
	}

	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"## Implement selector",
		"@id pm.context.impl",
		"@horizon now",
		"@accept cmd:test -f docs/specs/context-selection-v0.1.md",
		"",
		"Example only:",
		"",
		"```go",
		"// do not infer internal/context/selector.go from fenced examples",
		"```",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	compiled, err := compile.CompilePlan(planPath, []byte(planBody), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	packet, err := SelectByNeed(compiled, "pm.context.impl", NeedAuto)
	if err != nil {
		t.Fatalf("select by need: %v", err)
	}
	if got := packet.IncludedFiles; len(got) != 1 || got[0] != "docs/specs/context-selection-v0.1.md" {
		t.Fatalf("expected fenced path references to be ignored, got %#v", got)
	}
	if got := packet.IncludedFileRefs; len(got) != 1 || got[0].Path != "docs/specs/context-selection-v0.1.md" || got[0].Reason != "acceptance or scoped task text references repo files" {
		t.Fatalf("expected structured included file refs to ignore fenced paths, got %#v", got)
	}
	if len(packet.EscalationReasons) != 1 || packet.EscalationReasons[0] != "acceptance references repo files" {
		t.Fatalf("unexpected escalation reasons: %#v", packet.EscalationReasons)
	}
}

func TestSelectByNeedDependencyCheckIncludesClosureForDeclaredDeps(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	content := strings.Join([]string{
		"- [ ] Root task",
		"  @id task.root",
		"  @horizon now",
		"  @deps task.dep",
		"  @accept cmd:go test ./...",
		"",
		"- [ ] Dependency task",
		"  @id task.dep",
		"  @horizon now",
		"  @accept cmd:go test ./...",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	compiled, err := compile.CompilePlan(planPath, []byte(content), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	packet, err := SelectByNeed(compiled, "task.root", NeedDependencyCheck)
	if err != nil {
		t.Fatalf("select by need: %v", err)
	}
	if packet.SelectedContextClass != "task+deps" {
		t.Fatalf("expected task+deps, got %#v", packet)
	}
	if len(packet.Closure) != 1 || packet.Closure[0].TaskID != "task.dep" {
		t.Fatalf("expected dependency closure, got %#v", packet.Closure)
	}
	if len(packet.IncludedDepRefs) != 1 || packet.IncludedDepRefs[0].TaskID != "task.dep" || packet.IncludedDepRefs[0].Reason != "declared task dependencies require graph reasoning" {
		t.Fatalf("expected structured included dep refs, got %#v", packet.IncludedDepRefs)
	}
	if len(packet.EscalationReasons) != 1 || packet.EscalationReasons[0] != "declared task dependencies require graph reasoning" {
		t.Fatalf("unexpected dependency escalation reasons: %#v", packet.EscalationReasons)
	}
}

func TestSelectByNeedHandoffDoesNotAutoExpandDepsOnlyTask(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	content := strings.Join([]string{
		"- [ ] Root task",
		"  @id task.root",
		"  @horizon now",
		"  @deps task.dep",
		"  @accept cmd:go test ./...",
		"",
		"- [ ] Dependency task",
		"  @id task.dep",
		"  @horizon now",
		"  @accept cmd:go test ./...",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	compiled, err := compile.CompilePlan(planPath, []byte(content), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	packet, err := SelectByNeed(compiled, "task.root", NeedHandoff)
	if err != nil {
		t.Fatalf("select by need: %v", err)
	}
	if packet.SelectedContextClass != "task" {
		t.Fatalf("expected bounded handoff task packet, got %#v", packet)
	}
	if len(packet.Closure) != 0 {
		t.Fatalf("expected no dependency closure in default handoff packet, got %#v", packet.Closure)
	}
	if packet.NextUpgrade != "task+deps" {
		t.Fatalf("expected next upgrade task+deps, got %#v", packet)
	}
	if len(packet.IncludedDeps) != 1 || packet.IncludedDeps[0] != "task.dep" {
		t.Fatalf("expected compatibility included deps for handoff deps-only task, got %#v", packet.IncludedDeps)
	}
	if len(packet.IncludedDepRefs) != 1 || packet.IncludedDepRefs[0].TaskID != "task.dep" || packet.IncludedDepRefs[0].Reason != "declared task dependencies require graph reasoning" {
		t.Fatalf("expected structured dep refs for handoff deps-only task, got %#v", packet.IncludedDepRefs)
	}
	if len(packet.RemainingRisks) != 1 || !strings.Contains(packet.RemainingRisks[0], "dependency semantics omitted") {
		t.Fatalf("expected dependency omission risk, got %#v", packet.RemainingRisks)
	}
}

func TestSelectByNeedHandoffFilesAndDepsStaysFileBounded(t *testing.T) {
	tmp := t.TempDir()
	targetPath := filepath.Join(tmp, "docs", "specs", "context-selection-v0.1.md")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("mkdir target path: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("# spec\n"), 0o644); err != nil {
		t.Fatalf("write target file: %v", err)
	}

	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Root task",
		"  @id task.root",
		"  @horizon now",
		"  @deps task.dep",
		"  @accept cmd:test -f docs/specs/context-selection-v0.1.md",
		"",
		"- [ ] Dependency task",
		"  @id task.dep",
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

	packet, err := SelectByNeed(compiled, "task.root", NeedHandoff)
	if err != nil {
		t.Fatalf("select by need: %v", err)
	}
	if packet.SelectedContextClass != "task+files" {
		t.Fatalf("expected task+files handoff packet, got %#v", packet)
	}
	if len(packet.Closure) != 0 {
		t.Fatalf("expected no dependency closure in default handoff packet, got %#v", packet.Closure)
	}
	if packet.NextUpgrade != "task+files+deps" {
		t.Fatalf("expected next upgrade task+files+deps, got %#v", packet)
	}
	if len(packet.RemainingRisks) != 1 || !strings.Contains(packet.RemainingRisks[0], "dependency semantics omitted") {
		t.Fatalf("expected dependency omission risk, got %#v", packet.RemainingRisks)
	}
}

func TestBuildL0CachedMissThenHit(t *testing.T) {
	tmp := t.TempDir()
	stateDir := filepath.Join(tmp, ".planmark")
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

	packetA, hitA, err := BuildL0Cached(compiled, "fixture.task.now", stateDir)
	if err != nil {
		t.Fatalf("build L0 cached (first): %v", err)
	}
	if hitA {
		t.Fatalf("expected first cached build to miss")
	}

	packetB, hitB, err := BuildL0Cached(compiled, "fixture.task.now", stateDir)
	if err != nil {
		t.Fatalf("build L0 cached (second): %v", err)
	}
	if !hitB {
		t.Fatalf("expected second cached build to hit")
	}
	if packetA.SliceHash != packetB.SliceHash {
		t.Fatalf("expected cached packet to match initial packet slice hash")
	}
}

func TestBuildL2CachedMissThenHit(t *testing.T) {
	tmp := t.TempDir()
	stateDir := filepath.Join(tmp, ".planmark")
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Root task",
		"  @id fixture.task.root",
		"  @horizon later",
		"  @deps fixture.task.b,fixture.task.c",
		"",
		"- [ ] Dependency B",
		"  @id fixture.task.b",
		"  @horizon later",
		"  @deps fixture.task.c",
		"",
		"- [ ] Dependency C",
		"  @id fixture.task.c",
		"  @horizon later",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	compiled, err := compile.CompilePlan(planPath, []byte(planBody), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	packetA, hitA, err := BuildL2Cached(compiled, "fixture.task.root", stateDir)
	if err != nil {
		t.Fatalf("build L2 cached (first): %v", err)
	}
	if hitA {
		t.Fatalf("expected first cached build to miss")
	}
	if len(packetA.Closure) != 2 {
		t.Fatalf("expected L2 closure on first build, got %#v", packetA.Closure)
	}

	packetB, hitB, err := BuildL2Cached(compiled, "fixture.task.root", stateDir)
	if err != nil {
		t.Fatalf("build L2 cached (second): %v", err)
	}
	if !hitB {
		t.Fatalf("expected second cached build to hit")
	}
	if len(packetB.Closure) != 2 {
		t.Fatalf("expected L2 closure on cached build, got %#v", packetB.Closure)
	}
	if packetA.Closure[0].TaskID != packetB.Closure[0].TaskID || packetA.Closure[1].SliceHash != packetB.Closure[1].SliceHash {
		t.Fatalf("expected cached L2 packet to preserve deterministic closure")
	}
}

func TestNeedStatsDeterministicAcrossRepeatedSelection(t *testing.T) {
	tmp := t.TempDir()
	docPath := filepath.Join(tmp, "docs", "specs", "context-selection-v0.1.md")
	if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
		t.Fatalf("mkdir doc path: %v", err)
	}
	if err := os.WriteFile(docPath, []byte("# spec\n"), 0o644); err != nil {
		t.Fatalf("write doc file: %v", err)
	}

	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"## Root task",
		"@id fixture.task.root",
		"@horizon now",
		"@accept cmd:test -f docs/specs/context-selection-v0.1.md",
		"@deps fixture.task.dep",
		"",
		"## Dep task",
		"@id fixture.task.dep",
		"@horizon next",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	compiled, err := compile.CompilePlan(planPath, []byte(planBody), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	packetA, err := SelectByNeed(compiled, "fixture.task.root", NeedAuto)
	if err != nil {
		t.Fatalf("select by need (first): %v", err)
	}
	packetB, err := SelectByNeed(compiled, "fixture.task.root", NeedAuto)
	if err != nil {
		t.Fatalf("select by need (second): %v", err)
	}

	if !reflect.DeepEqual(packetA.Stats, packetB.Stats) {
		t.Fatalf("expected repeated selection stats to be deterministic:\nfirst=%#v\nsecond=%#v", packetA.Stats, packetB.Stats)
	}
	if packetA.SelectedContextClass != "task+files" {
		t.Fatalf("expected task+files packet, got %#v", packetA.SelectedContextClass)
	}
	if !reflect.DeepEqual(packetA.IncludedFileRefs, packetB.IncludedFileRefs) {
		t.Fatalf("expected repeated included file refs to match:\nfirst=%#v\nsecond=%#v", packetA.IncludedFileRefs, packetB.IncludedFileRefs)
	}
}

func TestNeedStatsRemainInternallyConsistentWhenPacketExceedsFullPlanBaseline(t *testing.T) {
	tmp := t.TempDir()
	docPath := filepath.Join(tmp, "docs", "specs", "context-selection-v0.1.md")
	if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
		t.Fatalf("mkdir doc path: %v", err)
	}
	specBody := strings.Repeat("# spec details\n", 80)
	if err := os.WriteFile(docPath, []byte(specBody), 0o644); err != nil {
		t.Fatalf("write doc file: %v", err)
	}

	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"## Eval task",
		"@id fixture.task.eval",
		"@horizon next",
		"@accept cmd:test -f docs/specs/context-selection-v0.1.md",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	compiled, err := compile.CompilePlan(planPath, []byte(planBody), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	packet, err := SelectByNeed(compiled, "fixture.task.eval", NeedAuto)
	if err != nil {
		t.Fatalf("select by need: %v", err)
	}

	if packet.Stats.FullPlanLines != countTextLines(planBody) {
		t.Fatalf("expected full plan line baseline %d, got %#v", countTextLines(planBody), packet.Stats.FullPlanLines)
	}
	if packet.Stats.IncludedFilesCount != 2 {
		t.Fatalf("expected plan slice plus pinned spec file, got %#v", packet.Stats.IncludedFilesCount)
	}
	if packet.Stats.SavedLinesVsFullPlan != packet.Stats.FullPlanLines-packet.Stats.IncludedLines {
		t.Fatalf("expected saved lines math to stay internally consistent, got %#v", packet.Stats)
	}
	if packet.Stats.SavedTokensVsFullPlan != packet.Stats.FullPlanEstimatedTokens-packet.Stats.EstimatedTokenCount {
		t.Fatalf("expected saved token math to stay internally consistent, got %#v", packet.Stats)
	}
	if packet.Stats.SavedLinesVsFullPlan >= 0 {
		t.Fatalf("expected oversized file-backed packet to show negative saved lines, got %#v", packet.Stats)
	}
}
