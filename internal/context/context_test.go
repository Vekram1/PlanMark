package context

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
