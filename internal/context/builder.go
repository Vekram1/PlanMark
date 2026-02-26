package context

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/cache"
	"github.com/vikramoddiraju/planmark/internal/ir"
)

var (
	ErrTaskNotFound = errors.New("task not found")
	ErrTaskNotReady = errors.New("task not ready for L0")
)

type L0Packet struct {
	Level      string   `json:"level"`
	TaskID     string   `json:"task_id"`
	NodeRef    string   `json:"node_ref"`
	Title      string   `json:"title"`
	Horizon    string   `json:"horizon,omitempty"`
	Deps       []string `json:"deps,omitempty"`
	Accept     []string `json:"accept,omitempty"`
	SourcePath string   `json:"source_path"`
	StartLine  int      `json:"start_line"`
	EndLine    int      `json:"end_line"`
	SliceHash  string   `json:"slice_hash"`
	SliceText  string   `json:"slice_text"`
}

type PinExtract struct {
	Key        string `json:"key"`
	TargetPath string `json:"target_path"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	TargetHash string `json:"target_hash"`
	SliceText  string `json:"slice_text"`
}

type L1Packet struct {
	L0Packet
	Pins []PinExtract `json:"pins,omitempty"`
}

func BuildL0(plan ir.PlanIR, taskID string) (L0Packet, error) {
	task, node, err := resolveTaskAndNode(plan, taskID)
	if err != nil {
		return L0Packet{}, err
	}
	return buildL0Packet(plan, task, node), nil
}

func BuildL1(plan ir.PlanIR, taskID string) (L1Packet, error) {
	task, node, err := resolveTaskAndNode(plan, taskID)
	if err != nil {
		return L1Packet{}, err
	}

	pins, err := extractPinTargets(plan, node)
	if err != nil {
		return L1Packet{}, err
	}
	base := buildL0Packet(plan, task, node)
	base.Level = "L1"
	return L1Packet{
		L0Packet: base,
		Pins:     pins,
	}, nil
}

func BuildL0Cached(plan ir.PlanIR, taskID string, stateDir string) (L0Packet, bool, error) {
	task, node, err := resolveTaskAndNode(plan, taskID)
	if err != nil {
		return L0Packet{}, false, err
	}
	key := cache.ContextPacketKey(cache.ContextKeyInput{
		Level:                           "L0",
		PlanPath:                        plan.PlanPath,
		IRVersion:                       plan.IRVersion,
		DeterminismPolicyVersion:        plan.DeterminismPolicyVersion,
		SemanticDerivationPolicyVersion: plan.SemanticDerivationPolicyVersion,
		TaskID:                          task.ID,
		TaskNodeRef:                     task.NodeRef,
		TaskSemanticFingerprint:         task.SemanticFingerprint,
		NodeSliceHash:                   node.SliceHash,
	})
	if strings.TrimSpace(stateDir) != "" {
		if payload, err := cache.ReadContextPacket(stateDir, key); err == nil {
			var packet L0Packet
			if err := json.Unmarshal(payload, &packet); err == nil {
				return packet, true, nil
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return L0Packet{}, false, err
		}
	}

	packet := buildL0Packet(plan, task, node)
	if strings.TrimSpace(stateDir) != "" {
		payload, err := json.Marshal(packet)
		if err != nil {
			return L0Packet{}, false, fmt.Errorf("marshal L0 packet: %w", err)
		}
		if _, err := cache.WriteContextPacket(stateDir, key, payload); err != nil {
			return L0Packet{}, false, err
		}
	}
	return packet, false, nil
}

func BuildL1Cached(plan ir.PlanIR, taskID string, stateDir string) (L1Packet, bool, error) {
	task, node, err := resolveTaskAndNode(plan, taskID)
	if err != nil {
		return L1Packet{}, false, err
	}
	pins, err := extractPinTargets(plan, node)
	if err != nil {
		return L1Packet{}, false, err
	}

	pinTargetHashes := make([]string, 0, len(pins))
	for _, pin := range pins {
		pinTargetHashes = append(pinTargetHashes, pin.TargetHash)
	}
	key := cache.ContextPacketKey(cache.ContextKeyInput{
		Level:                           "L1",
		PlanPath:                        plan.PlanPath,
		IRVersion:                       plan.IRVersion,
		DeterminismPolicyVersion:        plan.DeterminismPolicyVersion,
		SemanticDerivationPolicyVersion: plan.SemanticDerivationPolicyVersion,
		TaskID:                          task.ID,
		TaskNodeRef:                     task.NodeRef,
		TaskSemanticFingerprint:         task.SemanticFingerprint,
		NodeSliceHash:                   node.SliceHash,
		PinTargetHashes:                 pinTargetHashes,
	})
	if strings.TrimSpace(stateDir) != "" {
		if payload, err := cache.ReadContextPacket(stateDir, key); err == nil {
			var packet L1Packet
			if err := json.Unmarshal(payload, &packet); err == nil {
				return packet, true, nil
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return L1Packet{}, false, err
		}
	}

	base := buildL0Packet(plan, task, node)
	base.Level = "L1"
	packet := L1Packet{
		L0Packet: base,
		Pins:     pins,
	}
	if strings.TrimSpace(stateDir) != "" {
		payload, err := json.Marshal(packet)
		if err != nil {
			return L1Packet{}, false, fmt.Errorf("marshal L1 packet: %w", err)
		}
		if _, err := cache.WriteContextPacket(stateDir, key, payload); err != nil {
			return L1Packet{}, false, err
		}
	}
	return packet, false, nil
}

func resolveTaskAndNode(plan ir.PlanIR, taskID string) (ir.Task, ir.SourceNode, error) {
	requestedID := strings.TrimSpace(taskID)
	if requestedID == "" {
		return ir.Task{}, ir.SourceNode{}, fmt.Errorf("%w: empty id", ErrTaskNotFound)
	}

	var task ir.Task
	foundTask := false
	for _, candidate := range plan.Semantic.Tasks {
		if strings.TrimSpace(candidate.ID) == requestedID {
			task = candidate
			foundTask = true
			break
		}
	}
	if !foundTask {
		return ir.Task{}, ir.SourceNode{}, fmt.Errorf("%w: %s", ErrTaskNotFound, requestedID)
	}

	nodeByRef := make(map[string]ir.SourceNode, len(plan.Source.Nodes))
	for _, node := range plan.Source.Nodes {
		nodeByRef[node.NodeRef] = node
	}

	node, ok := nodeByRef[task.NodeRef]
	if !ok {
		return ir.Task{}, ir.SourceNode{}, fmt.Errorf("source node missing for task %q (node_ref=%s)", task.ID, task.NodeRef)
	}

	if strings.EqualFold(strings.TrimSpace(task.Horizon), "now") && !hasNonEmpty(task.Accept) {
		return ir.Task{}, ir.SourceNode{}, fmt.Errorf("%w: horizon=now task %q requires at least one @accept", ErrTaskNotReady, task.ID)
	}

	if strings.EqualFold(strings.TrimSpace(task.Horizon), "now") {
		taskByID := make(map[string]struct{}, len(plan.Semantic.Tasks))
		for _, candidate := range plan.Semantic.Tasks {
			taskByID[strings.TrimSpace(candidate.ID)] = struct{}{}
		}
		for _, dep := range task.Deps {
			depID := strings.TrimSpace(dep)
			if depID == "" {
				continue
			}
			if _, exists := taskByID[depID]; !exists {
				return ir.Task{}, ir.SourceNode{}, fmt.Errorf("%w: horizon=now task %q has unresolved dependency %q", ErrTaskNotReady, task.ID, depID)
			}
		}
	}

	return task, node, nil
}

func buildL0Packet(plan ir.PlanIR, task ir.Task, node ir.SourceNode) L0Packet {
	return L0Packet{
		Level:      "L0",
		TaskID:     task.ID,
		NodeRef:    task.NodeRef,
		Title:      task.Title,
		Horizon:    task.Horizon,
		Deps:       append([]string(nil), task.Deps...),
		Accept:     append([]string(nil), task.Accept...),
		SourcePath: plan.PlanPath,
		StartLine:  node.StartLine,
		EndLine:    node.EndLine,
		SliceHash:  node.SliceHash,
		SliceText:  node.SliceText,
	}
}

func hasNonEmpty(values []string) bool {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}
