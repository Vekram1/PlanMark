package cache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/compile"
	"github.com/vikramoddiraju/planmark/internal/ir"
)

func TestCacheKeyChangesOnSourceHashChange(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")

	basePlan := strings.Join([]string{
		"- [ ] Task now",
		"  @id fixture.task.now",
		"  @horizon now",
		"  @accept cmd:go test ./...",
	}, "\n")
	updatedPlan := strings.Replace(basePlan, "Task now", "Task now updated", 1)

	if err := os.WriteFile(planPath, []byte(basePlan), 0o644); err != nil {
		t.Fatalf("write base plan: %v", err)
	}
	compiledA, err := compile.CompilePlan(planPath, []byte(basePlan), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile base plan: %v", err)
	}

	if err := os.WriteFile(planPath, []byte(updatedPlan), 0o644); err != nil {
		t.Fatalf("write updated plan: %v", err)
	}
	compiledB, err := compile.CompilePlan(planPath, []byte(updatedPlan), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile updated plan: %v", err)
	}

	taskA, nodeA := lookupTaskAndNode(t, compiledA, "fixture.task.now")
	taskB, nodeB := lookupTaskAndNode(t, compiledB, "fixture.task.now")

	keyA := ContextPacketKey(ContextKeyInput{
		Level:                           "L0",
		PlanPath:                        compiledA.PlanPath,
		IRVersion:                       compiledA.IRVersion,
		DeterminismPolicyVersion:        compiledA.DeterminismPolicyVersion,
		SemanticDerivationPolicyVersion: compiledA.SemanticDerivationPolicyVersion,
		TaskID:                          taskA.ID,
		TaskNodeRef:                     taskA.NodeRef,
		TaskSemanticFingerprint:         taskA.SemanticFingerprint,
		NodeSliceHash:                   nodeA.SliceHash,
	})
	keyB := ContextPacketKey(ContextKeyInput{
		Level:                           "L0",
		PlanPath:                        compiledB.PlanPath,
		IRVersion:                       compiledB.IRVersion,
		DeterminismPolicyVersion:        compiledB.DeterminismPolicyVersion,
		SemanticDerivationPolicyVersion: compiledB.SemanticDerivationPolicyVersion,
		TaskID:                          taskB.ID,
		TaskNodeRef:                     taskB.NodeRef,
		TaskSemanticFingerprint:         taskB.SemanticFingerprint,
		NodeSliceHash:                   nodeB.SliceHash,
	})

	if keyA == keyB {
		t.Fatalf("expected different cache keys when source hash changes")
	}
}

func TestContextPacketKeyDeterministicForEquivalentInput(t *testing.T) {
	input := ContextKeyInput{
		Level:                           "l1",
		PlanPath:                        "a/b/PLAN.md",
		IRVersion:                       "v0.2",
		DeterminismPolicyVersion:        "v0.1",
		SemanticDerivationPolicyVersion: "v0.1",
		TaskID:                          "fixture.task",
		TaskNodeRef:                     "a/b/PLAN.md|checkbox|abc#1",
		TaskSemanticFingerprint:         "fp",
		NodeSliceHash:                   "hash",
		PinTargetHashes:                 []string{"c", "a", "b"},
	}

	keyA := ContextPacketKey(input)
	keyB := ContextPacketKey(input)
	if keyA != keyB {
		t.Fatalf("expected deterministic key for equivalent input")
	}

	input.PinTargetHashes = []string{"b", "a", "c"}
	keyC := ContextPacketKey(input)
	if keyA != keyC {
		t.Fatalf("expected pin hash order to be canonicalized")
	}
}

func TestContextPacketReadWriteRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	key := strings.Repeat("a", 64)
	payload := []byte("{\"level\":\"L0\"}\n")

	path, err := WriteContextPacket(tmp, key, payload)
	if err != nil {
		t.Fatalf("write context packet: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected cache file at %q: %v", path, err)
	}

	got, err := ReadContextPacket(tmp, key)
	if err != nil {
		t.Fatalf("read context packet: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload mismatch: got %q want %q", string(got), string(payload))
	}
}

func lookupTaskAndNode(t *testing.T, plan ir.PlanIR, taskID string) (ir.Task, ir.SourceNode) {
	t.Helper()

	var task ir.Task
	foundTask := false
	for _, candidate := range plan.Semantic.Tasks {
		if candidate.ID == taskID {
			task = candidate
			foundTask = true
			break
		}
	}
	if !foundTask {
		t.Fatalf("task %q not found", taskID)
	}

	for _, node := range plan.Source.Nodes {
		if node.NodeRef == task.NodeRef {
			return task, node
		}
	}
	t.Fatalf("node %q not found for task %q", task.NodeRef, taskID)
	return ir.Task{}, ir.SourceNode{}
}
