package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vikramoddiraju/planmark/internal/compile"
	"github.com/vikramoddiraju/planmark/internal/protocol"
)

func TestApplyChangeMetadataUpsertAndRecompileDoctor(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task now missing accept",
		"  @id fixture.task.now",
		"  @horizon now",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	compiled, err := compile.CompilePlan(planPath, []byte(planBody), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan fixture: %v", err)
	}
	task := compiled.Semantic.Tasks[0]
	var nodeSliceHash string
	var startLine int
	var endLine int
	for _, node := range compiled.Source.Nodes {
		if node.NodeRef == task.NodeRef {
			nodeSliceHash = node.SliceHash
			startLine = node.StartLine
			endLine = node.EndLine
			break
		}
	}

	delta := planDeltaDocument{
		SchemaVersion: planDeltaSchemaVersionV01,
		BasePlanHash:  sha256Hex([]byte(planBody)),
		Operations: []planDeltaOperation{
			{
				OpID: "op-001",
				Kind: "metadata_upsert",
				Target: planDeltaTarget{
					NodeRef: task.NodeRef,
					TaskID:  task.ID,
					Path:    filepath.ToSlash(planPath),
				},
				Precondition: planDeltaCondition{
					SourceHash: nodeSliceHash,
					SourceRange: planDeltaLineRange{
						StartLine: startLine,
						EndLine:   endLine,
					},
				},
				Payload: planDeltaPayload{
					Key:   "accept",
					Value: "cmd:echo ok",
				},
			},
		},
	}
	deltaPath := filepath.Join(tmp, "delta.json")
	rawDelta, _ := json.Marshal(delta)
	if err := os.WriteFile(deltaPath, rawDelta, 0o644); err != nil {
		t.Fatalf("write delta fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := runApplyChange([]string{deltaPath, "--plan", planPath, "--format", "json"}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected exit success, got %d stderr=%q", exit, errOut.String())
	}

	updatedRaw, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("read updated plan: %v", err)
	}
	if !strings.Contains(string(updatedRaw), "@accept cmd:echo ok") {
		t.Fatalf("expected inserted @accept metadata, got:\n%s", string(updatedRaw))
	}
}

func TestApplyChangeRejectsBaseHashMismatch(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := "- [ ] Task\n  @id fixture.task\n"
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}
	delta := planDeltaDocument{
		SchemaVersion: planDeltaSchemaVersionV01,
		BasePlanHash:  strings.Repeat("0", 64),
		Operations:    []planDeltaOperation{},
	}
	deltaPath := filepath.Join(tmp, "delta.json")
	rawDelta, _ := json.Marshal(delta)
	if err := os.WriteFile(deltaPath, rawDelta, 0o644); err != nil {
		t.Fatalf("write delta fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := runApplyChange([]string{deltaPath, "--plan", planPath}, &out, &errOut)
	if exit != protocol.ExitValidationFailed {
		t.Fatalf("expected validation failure, got %d", exit)
	}
	if !strings.Contains(errOut.String(), "base_hash_mismatch") {
		t.Fatalf("expected base_hash_mismatch message, got %q", errOut.String())
	}
}

func TestApplyChangeRejectsUnsupportedOperationKind(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := "- [ ] Task\n  @id fixture.task\n"
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}
	delta := planDeltaDocument{
		SchemaVersion: planDeltaSchemaVersionV01,
		BasePlanHash:  sha256Hex([]byte(planBody)),
		Operations: []planDeltaOperation{
			{
				OpID: "op-001",
				Kind: "replace",
				Target: planDeltaTarget{
					TaskID: "fixture.task",
				},
				Precondition: planDeltaCondition{
					SourceHash:  "abc",
					SourceRange: planDeltaLineRange{StartLine: 1, EndLine: 1},
				},
			},
		},
	}
	deltaPath := filepath.Join(tmp, "delta.json")
	rawDelta, _ := json.Marshal(delta)
	if err := os.WriteFile(deltaPath, rawDelta, 0o644); err != nil {
		t.Fatalf("write delta fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := runApplyChange([]string{deltaPath, "--plan", planPath}, &out, &errOut)
	if exit != protocol.ExitValidationFailed {
		t.Fatalf("expected validation failure, got %d", exit)
	}
	if !strings.Contains(errOut.String(), "operation_kind_unsupported") {
		t.Fatalf("expected unsupported-kind message, got %q", errOut.String())
	}
}

func TestApplyChangeRejectsPreconditionHashMismatch(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task",
		"  @id fixture.task",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	compiled, err := compile.CompilePlan(planPath, []byte(planBody), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan fixture: %v", err)
	}
	task := compiled.Semantic.Tasks[0]

	delta := planDeltaDocument{
		SchemaVersion: planDeltaSchemaVersionV01,
		BasePlanHash:  sha256Hex([]byte(planBody)),
		Operations: []planDeltaOperation{
			{
				OpID: "op-001",
				Kind: "metadata_upsert",
				Target: planDeltaTarget{
					TaskID: task.ID,
				},
				Precondition: planDeltaCondition{
					SourceHash: strings.Repeat("f", 64),
					SourceRange: planDeltaLineRange{
						StartLine: 1,
						EndLine:   1,
					},
				},
				Payload: planDeltaPayload{
					Key:   "accept",
					Value: "cmd:echo ok",
				},
			},
		},
	}
	deltaPath := filepath.Join(tmp, "delta.json")
	rawDelta, _ := json.Marshal(delta)
	if err := os.WriteFile(deltaPath, rawDelta, 0o644); err != nil {
		t.Fatalf("write delta fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := runApplyChange([]string{deltaPath, "--plan", planPath}, &out, &errOut)
	if exit != protocol.ExitValidationFailed {
		t.Fatalf("expected validation failure, got %d", exit)
	}
	if !strings.Contains(errOut.String(), "precondition_hash_mismatch") {
		t.Fatalf("expected precondition hash mismatch, got %q", errOut.String())
	}
}

func TestApplyChangeRejectsUnresolvedTarget(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task",
		"  @id fixture.task",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	delta := planDeltaDocument{
		SchemaVersion: planDeltaSchemaVersionV01,
		BasePlanHash:  sha256Hex([]byte(planBody)),
		Operations: []planDeltaOperation{
			{
				OpID: "op-001",
				Kind: "metadata_upsert",
				Target: planDeltaTarget{
					TaskID: "fixture.missing",
				},
				Precondition: planDeltaCondition{
					SourceHash: strings.Repeat("a", 64),
					SourceRange: planDeltaLineRange{
						StartLine: 1,
						EndLine:   2,
					},
				},
				Payload: planDeltaPayload{
					Key:   "accept",
					Value: "cmd:echo ok",
				},
			},
		},
	}
	deltaPath := filepath.Join(tmp, "delta.json")
	rawDelta, _ := json.Marshal(delta)
	if err := os.WriteFile(deltaPath, rawDelta, 0o644); err != nil {
		t.Fatalf("write delta fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := runApplyChange([]string{deltaPath, "--plan", planPath}, &out, &errOut)
	if exit != protocol.ExitValidationFailed {
		t.Fatalf("expected validation failure, got %d", exit)
	}
	if !strings.Contains(errOut.String(), "target_unresolved") {
		t.Fatalf("expected target_unresolved, got %q", errOut.String())
	}
}

func TestRunDispatchesApplyChange(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "PLAN.md")
	planBody := strings.Join([]string{
		"- [ ] Task now missing accept",
		"  @id fixture.task.now",
		"  @horizon now",
	}, "\n")
	if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}

	compiled, err := compile.CompilePlan(planPath, []byte(planBody), compile.NewParser(nil))
	if err != nil {
		t.Fatalf("compile plan fixture: %v", err)
	}
	task := compiled.Semantic.Tasks[0]
	var nodeSliceHash string
	var startLine int
	var endLine int
	for _, node := range compiled.Source.Nodes {
		if node.NodeRef == task.NodeRef {
			nodeSliceHash = node.SliceHash
			startLine = node.StartLine
			endLine = node.EndLine
			break
		}
	}

	delta := planDeltaDocument{
		SchemaVersion: planDeltaSchemaVersionV01,
		BasePlanHash:  sha256Hex([]byte(planBody)),
		Operations: []planDeltaOperation{
			{
				OpID: "op-001",
				Kind: "metadata_upsert",
				Target: planDeltaTarget{
					TaskID: task.ID,
				},
				Precondition: planDeltaCondition{
					SourceHash: nodeSliceHash,
					SourceRange: planDeltaLineRange{
						StartLine: startLine,
						EndLine:   endLine,
					},
				},
				Payload: planDeltaPayload{
					Key:   "accept",
					Value: "cmd:echo ok",
				},
			},
		},
	}
	deltaPath := filepath.Join(tmp, "delta.json")
	rawDelta, _ := json.Marshal(delta)
	if err := os.WriteFile(deltaPath, rawDelta, 0o644); err != nil {
		t.Fatalf("write delta fixture: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"apply-change", deltaPath, "--plan", planPath, "--format", "json"}, &out, &errOut)
	if exit != protocol.ExitSuccess {
		t.Fatalf("expected dispatch success, got %d stderr=%q", exit, errOut.String())
	}
}
