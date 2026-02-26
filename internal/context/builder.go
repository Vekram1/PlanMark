package context

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/cache"
	"github.com/vikramoddiraju/planmark/internal/fsio"
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
	Key         string `json:"key"`
	TargetPath  string `json:"target_path"`
	StartLine   int    `json:"start_line"`
	EndLine     int    `json:"end_line"`
	TargetHash  string `json:"target_hash"`
	Freshness   string `json:"freshness"`
	Baseline    string `json:"baseline_target_hash,omitempty"`
	SliceText   string `json:"slice_text"`
	identityEnd int    `json:"-"`
}

type L1Packet struct {
	L0Packet
	Pins []PinExtract `json:"pins,omitempty"`
}

type L2Dependency struct {
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
}

type L2Packet struct {
	L0Packet
	Closure []L2Dependency `json:"closure,omitempty"`
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

func BuildL2(plan ir.PlanIR, taskID string) (L2Packet, error) {
	task, node, err := resolveTaskAndNode(plan, taskID)
	if err != nil {
		return L2Packet{}, err
	}

	taskByID := make(map[string]ir.Task, len(plan.Semantic.Tasks))
	for _, candidate := range plan.Semantic.Tasks {
		taskByID[strings.TrimSpace(candidate.ID)] = candidate
	}
	nodeByRef := make(map[string]ir.SourceNode, len(plan.Source.Nodes))
	for _, sourceNode := range plan.Source.Nodes {
		nodeByRef[sourceNode.NodeRef] = sourceNode
	}

	visited := make(map[string]struct{})
	var visit func(string) error
	visit = func(id string) error {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil
		}
		if _, seen := visited[id]; seen {
			return nil
		}
		depTask, ok := taskByID[id]
		if !ok {
			return fmt.Errorf("dependency task not found: %s", id)
		}
		visited[id] = struct{}{}

		nextDeps := append([]string(nil), depTask.Deps...)
		sort.Strings(nextDeps)
		for _, depID := range nextDeps {
			if err := visit(depID); err != nil {
				return err
			}
		}
		return nil
	}

	rootDeps := append([]string(nil), task.Deps...)
	sort.Strings(rootDeps)
	for _, depID := range rootDeps {
		if err := visit(depID); err != nil {
			return L2Packet{}, err
		}
	}

	orderedIDs := make([]string, 0, len(visited))
	for id := range visited {
		orderedIDs = append(orderedIDs, id)
	}
	sort.Strings(orderedIDs)

	closure := make([]L2Dependency, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		depTask := taskByID[id]
		depNode, ok := nodeByRef[depTask.NodeRef]
		if !ok {
			return L2Packet{}, fmt.Errorf("source node missing for dependency task %q (node_ref=%s)", depTask.ID, depTask.NodeRef)
		}
		closure = append(closure, L2Dependency{
			TaskID:     depTask.ID,
			NodeRef:    depTask.NodeRef,
			Title:      depTask.Title,
			Horizon:    depTask.Horizon,
			Deps:       append([]string(nil), depTask.Deps...),
			Accept:     append([]string(nil), depTask.Accept...),
			SourcePath: plan.PlanPath,
			StartLine:  depNode.StartLine,
			EndLine:    depNode.EndLine,
			SliceHash:  depNode.SliceHash,
		})
	}

	base := buildL0Packet(plan, task, node)
	base.Level = "L2"
	return L2Packet{
		L0Packet: base,
		Closure:  closure,
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
	baseKey := cache.ContextPacketKey(cache.ContextKeyInput{
		Level:                           "L1",
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
		annotatedPins, err := applyPinFreshnessBaseline(stateDir, baseKey, pins)
		if err != nil {
			return L1Packet{}, false, err
		}
		pins = annotatedPins
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

type pinFreshnessBaseline struct {
	Pins map[string]string `json:"pins"`
}

func applyPinFreshnessBaseline(stateDir string, baseKey string, pins []PinExtract) ([]PinExtract, error) {
	baselinePath := filepath.Join(stateDir, "cache", "context", "l1-freshness", baseKey+".json")
	payload, err := os.ReadFile(baselinePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			for i := range pins {
				pins[i].Freshness = "fresh"
			}
			if writeErr := writePinFreshnessBaseline(baselinePath, pins); writeErr != nil {
				return nil, writeErr
			}
			return pins, nil
		}
		return nil, fmt.Errorf("read pin freshness baseline: %w", err)
	}

	var baseline pinFreshnessBaseline
	if err := json.Unmarshal(payload, &baseline); err != nil {
		return nil, fmt.Errorf("decode pin freshness baseline: %w", err)
	}

	for i := range pins {
		id := pinIdentity(pins[i])
		previousHash, ok := baseline.Pins[id]
		if !ok {
			pins[i].Freshness = "unknown"
			continue
		}
		if previousHash != pins[i].TargetHash {
			pins[i].Freshness = "stale"
			pins[i].Baseline = previousHash
			continue
		}
		pins[i].Freshness = "fresh"
	}
	return pins, nil
}

func writePinFreshnessBaseline(path string, pins []PinExtract) error {
	baseline := pinFreshnessBaseline{Pins: map[string]string{}}
	for _, pin := range pins {
		baseline.Pins[pinIdentity(pin)] = pin.TargetHash
	}
	payload, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal pin freshness baseline: %w", err)
	}
	payload = append(payload, '\n')
	if err := fsio.WriteFileAtomic(path, payload, 0o644); err != nil {
		return fmt.Errorf("write pin freshness baseline: %w", err)
	}
	return nil
}

func pinIdentity(pin PinExtract) string {
	return fmt.Sprintf("%s|%s|%d|%d", pin.Key, pin.TargetPath, pin.StartLine, pin.identityEnd)
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
