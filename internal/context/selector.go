package context

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/ir"
)

type Need string

const (
	NeedExecute         Need = "execute"
	NeedEdit            Need = "edit"
	NeedDependencyCheck Need = "dependency-check"
	NeedHandoff         Need = "handoff"
	NeedAuto            Need = "auto"
)

type NeedPacket struct {
	L0Packet
	Need                 string         `json:"need"`
	SelectedContextClass string         `json:"selected_context_class"`
	SufficientForNeed    bool           `json:"sufficient_for_need"`
	EscalationReasons    []string       `json:"escalation_reasons,omitempty"`
	IncludedFiles        []string       `json:"included_files,omitempty"`
	IncludedDeps         []string       `json:"included_deps,omitempty"`
	RemainingRisks       []string       `json:"remaining_risks,omitempty"`
	NextUpgrade          string         `json:"next_upgrade,omitempty"`
	Pins                 []PinExtract   `json:"pins,omitempty"`
	Closure              []L2Dependency `json:"closure,omitempty"`
}

func ParseNeed(raw string) (Need, error) {
	normalized := Need(strings.ToLower(strings.TrimSpace(raw)))
	switch normalized {
	case NeedExecute, NeedEdit, NeedDependencyCheck, NeedHandoff, NeedAuto:
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid need %q (supported: execute|edit|dependency-check|handoff|auto)", raw)
	}
}

func SelectByNeed(plan ir.PlanIR, taskID string, need Need) (NeedPacket, error) {
	task, node, err := resolveTaskAndNode(plan, taskID)
	if err != nil {
		return NeedPacket{}, err
	}

	base := buildL0Packet(plan, task, node)
	packet := NeedPacket{
		L0Packet:             base,
		Need:                 string(need),
		SelectedContextClass: "task",
		SufficientForNeed:    true,
	}

	pins, fileReasons, pinErr := collectFileEvidence(plan, task, node, base)
	hasFileEvidence := pinErr == nil && len(pins) > 0
	hasDeps := len(task.Deps) > 0

	switch need {
	case NeedExecute:
		packet.NextUpgrade = "task+files"
	case NeedEdit:
		if pinErr != nil {
			return NeedPacket{}, pinErr
		}
		if hasFileEvidence {
			packet.SelectedContextClass = "task+files"
			packet.Level = "L1"
			packet.Pins = pins
			packet.EscalationReasons = append([]string(nil), fileReasons...)
			packet.IncludedFiles = pinTargetPaths(pins)
			if hasDeps {
				packet.NextUpgrade = "task+files+deps"
			}
		} else {
			packet.NextUpgrade = "task+files"
		}
	case NeedDependencyCheck:
		if hasDeps {
			l2, err := BuildL2(plan, taskID)
			if err != nil {
				return NeedPacket{}, err
			}
			packet.L0Packet = l2.L0Packet
			packet.SelectedContextClass = "task+deps"
			packet.Closure = l2.Closure
			packet.EscalationReasons = []string{"declared task dependencies require graph reasoning"}
			packet.IncludedDeps = append([]string(nil), task.Deps...)
			if hasFileEvidence {
				packet.NextUpgrade = "task+files+deps"
			}
		} else {
			packet.NextUpgrade = "task+deps"
		}
	case NeedHandoff:
		if pinErr != nil {
			return NeedPacket{}, pinErr
		}
		switch {
		case hasFileEvidence && hasDeps:
			packet.Level = "L1"
			packet.SelectedContextClass = "task+files"
			packet.Pins = pins
			packet.EscalationReasons = append([]string(nil), fileReasons...)
			packet.IncludedFiles = pinTargetPaths(pins)
			packet.NextUpgrade = "task+files+deps"
			packet.RemainingRisks = []string{"dependency semantics omitted from handoff packet; upgrade to task+files+deps when ordering or blocker analysis is required"}
		case hasFileEvidence:
			packet.Level = "L1"
			packet.SelectedContextClass = "task+files"
			packet.Pins = pins
			packet.EscalationReasons = append([]string(nil), fileReasons...)
			packet.IncludedFiles = pinTargetPaths(pins)
			packet.NextUpgrade = "task+files+deps"
		case hasDeps:
			packet.NextUpgrade = "task+deps"
			packet.RemainingRisks = []string{"dependency semantics omitted from handoff packet; upgrade to task+deps when ordering or blocker analysis is required"}
		default:
			packet.NextUpgrade = "task+files+deps"
		}
	case NeedAuto:
		if pinErr != nil {
			return NeedPacket{}, pinErr
		}
		switch {
		case hasFileEvidence:
			packet.Level = "L1"
			if hasDeps {
				packet.SelectedContextClass = "task+files"
				packet.NextUpgrade = "task+files+deps"
			} else {
				packet.SelectedContextClass = "task+files"
			}
			packet.Pins = pins
			packet.EscalationReasons = append([]string(nil), fileReasons...)
			packet.IncludedFiles = pinTargetPaths(pins)
		case hasDeps:
			packet.NextUpgrade = "task+deps"
		default:
			packet.NextUpgrade = "task+files"
		}
	}

	return packet, nil
}

func collectFileEvidence(plan ir.PlanIR, task ir.Task, node ir.SourceNode, base L0Packet) ([]PinExtract, []string, error) {
	explicitPins, err := extractPinTargets(plan, node)
	if err != nil {
		return nil, nil, err
	}

	pins := append([]PinExtract(nil), explicitPins...)
	reasons := make([]string, 0, 3)
	if len(explicitPins) > 0 {
		reasons = append(reasons, "explicit file metadata present")
	}

	inferredPins, inferredReasons, err := inferRepoFilePins(plan, task, base)
	if err != nil {
		return nil, nil, err
	}
	pins = appendUniquePinsByTarget(pins, inferredPins...)
	reasons = appendUniqueStrings(reasons, inferredReasons...)
	return pins, reasons, nil
}

func inferRepoFilePins(plan ir.PlanIR, task ir.Task, base L0Packet) ([]PinExtract, []string, error) {
	repoRoot := strings.TrimSpace(plan.PlanPath)
	if repoRoot == "" {
		return nil, nil, nil
	}
	repoRoot = filepathDir(repoRoot)

	candidates := make(map[string]struct{})
	fromAccept := false
	fromSections := false
	for _, accept := range task.Accept {
		paths := findRepoFileCandidates(repoRoot, accept)
		if len(paths) > 0 {
			fromAccept = true
		}
		for _, path := range paths {
			candidates[path] = struct{}{}
		}
	}
	for _, section := range base.Sections {
		if strings.TrimSpace(section.Title) != "" {
			for _, path := range findRepoFileCandidates(repoRoot, section.Title) {
				candidates[path] = struct{}{}
				fromSections = true
			}
		}
		inFence := false
		for _, line := range section.Body {
			if fenceToggleLine(line) {
				inFence = !inFence
				continue
			}
			if inFence {
				continue
			}
			paths := findRepoFileCandidates(repoRoot, line)
			if len(paths) > 0 {
				fromSections = true
			}
			for _, path := range paths {
				candidates[path] = struct{}{}
			}
		}
	}

	ordered := make([]string, 0, len(candidates))
	for path := range candidates {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)

	pins := make([]PinExtract, 0, len(ordered))
	for _, path := range ordered {
		pin, err := buildPinExtract(repoRoot, "inferred", path)
		if err != nil {
			return nil, nil, err
		}
		pins = append(pins, pin)
	}

	reasons := make([]string, 0, 2)
	if fromAccept {
		reasons = append(reasons, "acceptance references repo files")
	}
	if fromSections {
		reasons = append(reasons, "scoped task text references repo files")
	}
	return pins, reasons, nil
}

func findRepoFileCandidates(repoRoot string, text string) []string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return nil
	}
	out := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		candidate := normalizeRepoPathToken(field)
		if candidate == "" {
			continue
		}
		normalized, absPath, err := resolveRepoScopedPath(repoRoot, candidate)
		if err != nil {
			continue
		}
		info, err := os.Stat(absPath)
		if err != nil || info.IsDir() {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}

func normalizeRepoPathToken(token string) string {
	candidate := strings.TrimSpace(token)
	candidate = strings.Trim(candidate, "\"'`()[]{}<>,;")
	candidate = strings.TrimSuffix(candidate, ":")
	if candidate == "" {
		return ""
	}
	if strings.Contains(candidate, "\\") {
		return ""
	}
	if strings.HasPrefix(candidate, "@") {
		return ""
	}
	if !strings.Contains(candidate, ".") && !strings.Contains(candidate, "/") {
		return ""
	}
	if strings.HasPrefix(candidate, "cmd:") || strings.HasPrefix(candidate, "http://") || strings.HasPrefix(candidate, "https://") {
		return ""
	}
	if strings.Contains(candidate, ":") {
		return ""
	}
	if strings.HasSuffix(candidate, "/") {
		return ""
	}
	return candidate
}

func appendUniquePinsByTarget(existing []PinExtract, more ...PinExtract) []PinExtract {
	seen := make(map[string]struct{}, len(existing))
	for _, pin := range existing {
		path := strings.TrimSpace(pin.TargetPath)
		if path == "" {
			continue
		}
		seen[path] = struct{}{}
	}
	for _, pin := range more {
		path := strings.TrimSpace(pin.TargetPath)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		existing = append(existing, pin)
	}
	return existing
}

func appendUniqueStrings(existing []string, more ...string) []string {
	seen := make(map[string]struct{}, len(existing))
	for _, item := range existing {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		seen[item] = struct{}{}
	}
	for _, item := range more {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		existing = append(existing, item)
	}
	return existing
}

func fenceToggleLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
}

func filepathDir(path string) string {
	idx := strings.LastIndex(path, "/")
	if idx <= 0 {
		return "."
	}
	return path[:idx]
}

func pinTargetPaths(pins []PinExtract) []string {
	out := make([]string, 0, len(pins))
	seen := make(map[string]struct{}, len(pins))
	for _, pin := range pins {
		path := strings.TrimSpace(pin.TargetPath)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out
}
