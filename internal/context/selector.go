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
	Query                 string         `json:"query"`
	Need                  string         `json:"need"`
	SelectedContextClass  string         `json:"selected_context_class"`
	SufficientForNeed     bool           `json:"sufficient_for_need"`
	FallbackUsed          bool           `json:"fallback_used"`
	FullPlanRequired      bool           `json:"full_plan_required"`
	EscalationReasons     []string       `json:"escalation_reasons,omitempty"`
	IncludedFiles         []string       `json:"included_files,omitempty"`
	IncludedFileRefs      []IncludedFile `json:"included_file_refs,omitempty"`
	IncludedDeps          []string       `json:"included_deps,omitempty"`
	IncludedDepRefs       []IncludedDep  `json:"included_dep_refs,omitempty"`
	IncludedDependents    []string       `json:"included_dependents,omitempty"`
	IncludedDependentRefs []IncludedDep  `json:"included_dependent_refs,omitempty"`
	RemainingRisks        []string       `json:"remaining_risks,omitempty"`
	NextUpgrade           string         `json:"next_upgrade,omitempty"`
	Pins                  []PinExtract   `json:"pins,omitempty"`
	Dependencies          []L2Dependency `json:"dependencies,omitempty"`
	Dependents            []L2Dependency `json:"dependents,omitempty"`
	Closure               []L2Dependency `json:"closure,omitempty"`
	Stats                 ContextStats   `json:"stats"`
}

type IncludedFile struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type IncludedDep struct {
	TaskID string `json:"task_id"`
	Reason string `json:"reason"`
}

type ContextStats struct {
	IncludedLines           int      `json:"included_lines"`
	IncludedFilesCount      int      `json:"included_files_count"`
	IncludedDepsCount       int      `json:"included_deps_count"`
	EstimatedTokenCount     int      `json:"estimated_token_count"`
	EscalationPath          []string `json:"escalation_path"`
	FullPlanLines           int      `json:"full_plan_lines,omitempty"`
	FullPlanEstimatedTokens int      `json:"full_plan_estimated_tokens,omitempty"`
	SavedLinesVsFullPlan    int      `json:"saved_lines_vs_full_plan,omitempty"`
	SavedTokensVsFullPlan   int      `json:"saved_tokens_vs_full_plan,omitempty"`
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
		Query:                strings.TrimSpace(taskID),
		Need:                 string(need),
		SelectedContextClass: "task",
		SufficientForNeed:    true,
		FallbackUsed:         false,
		FullPlanRequired:     false,
	}

	pins, fileReasons, pinErr := collectFileEvidence(plan, task, node, base)
	hasFileEvidence := pinErr == nil && len(pins) > 0
	hasDeps := len(task.Deps) > 0
	dependencies, dependents, neighborhoodErr := collectGraphNeighborhood(plan, task)
	if neighborhoodErr != nil {
		return NeedPacket{}, neighborhoodErr
	}
	hasDirectDeps := len(dependencies) > 0
	hasDependents := len(dependents) > 0
	hasNeighborhood := hasDirectDeps || hasDependents

	switch need {
	case NeedExecute:
		packet.NextUpgrade = "task+files"
	case NeedEdit:
		if pinErr != nil {
			return NeedPacket{}, pinErr
		}
		if hasNeighborhood {
			packet.Level = "L2"
			packet.Dependencies = append([]L2Dependency(nil), dependencies...)
			packet.Dependents = append([]L2Dependency(nil), dependents...)
			if hasDirectDeps {
				packet.IncludedDeps = taskIDsFromSummaries(dependencies)
				packet.IncludedDepRefs = buildIncludedDepRefs(packet.IncludedDeps, "direct upstream dependencies constrain task edits")
				packet.EscalationReasons = append(packet.EscalationReasons, "direct upstream dependencies constrain task edits")
			}
			if hasDependents {
				packet.IncludedDependents = taskIDsFromSummaries(dependents)
				packet.IncludedDependentRefs = buildIncludedDepRefs(packet.IncludedDependents, "direct downstream tasks may be impacted by task edits")
				packet.EscalationReasons = append(packet.EscalationReasons, "direct downstream tasks may be impacted by task edits")
			}
		}
		if hasFileEvidence {
			packet.Pins = pins
			packet.EscalationReasons = append(packet.EscalationReasons, fileReasons...)
			packet.IncludedFiles = pinTargetPaths(pins)
			packet.IncludedFileRefs = buildIncludedFileRefs(pins)
		}
		switch {
		case hasFileEvidence && hasDirectDeps && hasDependents:
			packet.SelectedContextClass = "task+files+deps+dependents"
			packet.NextUpgrade = "task+files+deps-closure"
		case hasFileEvidence && hasDirectDeps:
			packet.SelectedContextClass = "task+files+deps"
			packet.NextUpgrade = "task+files+deps-closure"
		case hasFileEvidence && hasDependents:
			packet.SelectedContextClass = "task+files+dependents"
			packet.NextUpgrade = "task+files"
		case hasDirectDeps && hasDependents:
			packet.SelectedContextClass = "task+deps+dependents"
			packet.NextUpgrade = "task+files+deps-closure"
		case hasDirectDeps:
			packet.SelectedContextClass = "task+deps"
			packet.NextUpgrade = "task+files+deps-closure"
		case hasDependents:
			packet.SelectedContextClass = "task+dependents"
			packet.NextUpgrade = "task+files"
		case hasFileEvidence:
			packet.SelectedContextClass = "task+files"
			packet.Level = "L1"
			packet.NextUpgrade = "task+files+deps+dependents"
		default:
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
			packet.IncludedDepRefs = buildIncludedDepRefs(task.Deps, "declared task dependencies require graph reasoning")
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
		if hasNeighborhood {
			packet.Level = "L2"
			packet.Dependencies = append([]L2Dependency(nil), dependencies...)
			packet.Dependents = append([]L2Dependency(nil), dependents...)
			if hasDirectDeps {
				packet.IncludedDeps = taskIDsFromSummaries(dependencies)
				packet.IncludedDepRefs = buildIncludedDepRefs(packet.IncludedDeps, "direct upstream dependencies constrain handoff ordering")
				packet.EscalationReasons = append(packet.EscalationReasons, "direct upstream dependencies constrain handoff ordering")
			}
			if hasDependents {
				packet.IncludedDependents = taskIDsFromSummaries(dependents)
				packet.IncludedDependentRefs = buildIncludedDepRefs(packet.IncludedDependents, "direct downstream tasks depend on this handoff target")
				packet.EscalationReasons = append(packet.EscalationReasons, "direct downstream tasks depend on this handoff target")
			}
		}
		if hasFileEvidence {
			packet.Pins = pins
			packet.EscalationReasons = append(packet.EscalationReasons, fileReasons...)
			packet.IncludedFiles = pinTargetPaths(pins)
			packet.IncludedFileRefs = buildIncludedFileRefs(pins)
		}
		switch {
		case hasFileEvidence && hasDirectDeps && hasDependents:
			packet.SelectedContextClass = "task+files+deps+dependents"
		case hasFileEvidence && hasDirectDeps:
			packet.SelectedContextClass = "task+files+deps"
		case hasFileEvidence && hasDependents:
			packet.SelectedContextClass = "task+files+dependents"
		case hasDirectDeps && hasDependents:
			packet.SelectedContextClass = "task+deps+dependents"
		case hasDirectDeps:
			packet.SelectedContextClass = "task+deps"
		case hasDependents:
			packet.SelectedContextClass = "task+dependents"
		case hasFileEvidence:
			packet.SelectedContextClass = "task+files"
			packet.Level = "L1"
		}
		switch {
		case hasDirectDeps && directDependenciesHaveOwnDeps(dependencies):
			packet.NextUpgrade = "task+deps-closure"
			if hasFileEvidence {
				packet.NextUpgrade = "task+files+deps-closure"
			}
			packet.RemainingRisks = []string{"transitive upstream dependency closure omitted from bounded handoff packet; upgrade when blocker-chain analysis is required"}
		case hasFileEvidence && !hasNeighborhood:
			packet.NextUpgrade = "task+files+deps+dependents"
		case hasDirectDeps || hasDependents:
			packet.NextUpgrade = "task+files+deps+dependents"
		default:
			packet.NextUpgrade = "task+files+deps+dependents"
		}
	case NeedAuto:
		if pinErr != nil {
			return NeedPacket{}, pinErr
		}
		switch {
		case hasFileEvidence && hasNeighborhood:
			packet.Level = "L2"
			packet.Pins = pins
			packet.EscalationReasons = append([]string(nil), fileReasons...)
			packet.IncludedFiles = pinTargetPaths(pins)
			packet.IncludedFileRefs = buildIncludedFileRefs(pins)
			packet.Dependencies = append([]L2Dependency(nil), dependencies...)
			packet.Dependents = append([]L2Dependency(nil), dependents...)
			packet.IncludedDeps = taskIDsFromSummaries(dependencies)
			packet.IncludedDepRefs = buildIncludedDepRefs(packet.IncludedDeps, "direct upstream dependencies constrain immediate work")
			packet.IncludedDependents = taskIDsFromSummaries(dependents)
			packet.IncludedDependentRefs = buildIncludedDepRefs(packet.IncludedDependents, "direct downstream tasks may be impacted by immediate work")
			packet.SelectedContextClass = composeGraphContextClass(true, hasDirectDeps, hasDependents)
			packet.NextUpgrade = "task+files+deps-closure"
		case hasFileEvidence:
			packet.Level = "L1"
			packet.SelectedContextClass = "task+files"
			packet.Pins = pins
			packet.EscalationReasons = append([]string(nil), fileReasons...)
			packet.IncludedFiles = pinTargetPaths(pins)
			packet.IncludedFileRefs = buildIncludedFileRefs(pins)
			if hasDeps || hasDependents {
				packet.NextUpgrade = "task+files+deps+dependents"
			}
		case hasDeps:
			packet.Level = "L2"
			packet.SelectedContextClass = composeGraphContextClass(false, hasDirectDeps, hasDependents)
			packet.Dependencies = append([]L2Dependency(nil), dependencies...)
			packet.Dependents = append([]L2Dependency(nil), dependents...)
			packet.IncludedDeps = taskIDsFromSummaries(dependencies)
			packet.IncludedDepRefs = buildIncludedDepRefs(packet.IncludedDeps, "direct upstream dependencies constrain immediate work")
			packet.IncludedDependents = taskIDsFromSummaries(dependents)
			packet.IncludedDependentRefs = buildIncludedDepRefs(packet.IncludedDependents, "direct downstream tasks may be impacted by immediate work")
			packet.NextUpgrade = "task+deps"
		case hasDependents:
			packet.Level = "L2"
			packet.SelectedContextClass = "task+dependents"
			packet.Dependents = append([]L2Dependency(nil), dependents...)
			packet.IncludedDependents = taskIDsFromSummaries(dependents)
			packet.IncludedDependentRefs = buildIncludedDepRefs(packet.IncludedDependents, "direct downstream tasks may be impacted by immediate work")
			packet.NextUpgrade = "task+files"
		default:
			packet.NextUpgrade = "task+files"
		}
	}

	packet.Stats = buildContextStats(plan, packet)
	return packet, nil
}

func buildContextStats(plan ir.PlanIR, packet NeedPacket) ContextStats {
	stats := ContextStats{
		IncludedLines:       packetLineCount(packet),
		IncludedFilesCount:  packetFileCount(packet),
		IncludedDepsCount:   packetDepCount(packet),
		EstimatedTokenCount: estimateTokens(packetTokenText(packet)),
		EscalationPath:      escalationPath(packet.SelectedContextClass),
	}

	fullPlanText, fullPlanLines := readFullPlanStats(plan)
	stats.FullPlanLines = fullPlanLines
	stats.FullPlanEstimatedTokens = estimateTokens(fullPlanText)
	if stats.FullPlanLines > 0 {
		stats.SavedLinesVsFullPlan = stats.FullPlanLines - stats.IncludedLines
	}
	if stats.FullPlanEstimatedTokens > 0 {
		stats.SavedTokensVsFullPlan = stats.FullPlanEstimatedTokens - stats.EstimatedTokenCount
	}
	return stats
}

func packetLineCount(packet NeedPacket) int {
	total := lineRangeSize(packet.StartLine, packet.EndLine)
	for _, pin := range packet.Pins {
		total += lineRangeSize(pin.StartLine, pin.EndLine)
	}
	for _, dep := range packet.Dependencies {
		total += lineRangeSize(dep.StartLine, dep.EndLine)
	}
	for _, dep := range packet.Closure {
		total += lineRangeSize(dep.StartLine, dep.EndLine)
	}
	for _, dep := range packet.Dependents {
		total += lineRangeSize(dep.StartLine, dep.EndLine)
	}
	return total
}

func packetFileCount(packet NeedPacket) int {
	paths := make(map[string]struct{})
	if path := strings.TrimSpace(packet.SourcePath); path != "" {
		paths[path] = struct{}{}
	}
	for _, pin := range packet.Pins {
		if path := strings.TrimSpace(pin.TargetPath); path != "" {
			paths[path] = struct{}{}
		}
	}
	for _, dep := range packet.Dependencies {
		if path := strings.TrimSpace(dep.SourcePath); path != "" {
			paths[path] = struct{}{}
		}
	}
	for _, dep := range packet.Closure {
		if path := strings.TrimSpace(dep.SourcePath); path != "" {
			paths[path] = struct{}{}
		}
	}
	for _, dep := range packet.Dependents {
		if path := strings.TrimSpace(dep.SourcePath); path != "" {
			paths[path] = struct{}{}
		}
	}
	return len(paths)
}

func packetDepCount(packet NeedPacket) int {
	total := len(packet.Dependencies) + len(packet.Closure) + len(packet.Dependents)
	if total > 0 {
		return total
	}
	if len(packet.Closure) > 0 {
		return len(packet.Closure)
	}
	if len(packet.IncludedDeps) > 0 || len(packet.IncludedDependents) > 0 {
		return len(packet.IncludedDeps) + len(packet.IncludedDependents)
	}
	return 0
}

func packetTokenText(packet NeedPacket) string {
	parts := make([]string, 0, 1+len(packet.Pins)+len(packet.Dependencies)+len(packet.Closure)+len(packet.Dependents))
	if text := strings.TrimSpace(packet.SliceText); text != "" {
		parts = append(parts, text)
	}
	for _, pin := range packet.Pins {
		if text := strings.TrimSpace(pin.SliceText); text != "" {
			parts = append(parts, text)
		}
	}
	for _, dep := range packet.Dependencies {
		summary := strings.Join([]string{
			strings.TrimSpace(dep.TaskID),
			strings.TrimSpace(dep.Title),
			strings.Join(dep.Deps, " "),
			strings.Join(dep.Accept, " "),
		}, "\n")
		if text := strings.TrimSpace(summary); text != "" {
			parts = append(parts, text)
		}
	}
	for _, dep := range packet.Closure {
		summary := strings.Join([]string{
			strings.TrimSpace(dep.TaskID),
			strings.TrimSpace(dep.Title),
			strings.Join(dep.Deps, " "),
			strings.Join(dep.Accept, " "),
		}, "\n")
		if text := strings.TrimSpace(summary); text != "" {
			parts = append(parts, text)
		}
	}
	for _, dep := range packet.Dependents {
		summary := strings.Join([]string{
			strings.TrimSpace(dep.TaskID),
			strings.TrimSpace(dep.Title),
			strings.Join(dep.Deps, " "),
			strings.Join(dep.Accept, " "),
		}, "\n")
		if text := strings.TrimSpace(summary); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func collectGraphNeighborhood(plan ir.PlanIR, task ir.Task) ([]L2Dependency, []L2Dependency, error) {
	taskByID := make(map[string]ir.Task, len(plan.Semantic.Tasks))
	nodeByRef := make(map[string]ir.SourceNode, len(plan.Source.Nodes))
	for _, candidate := range plan.Semantic.Tasks {
		taskByID[strings.TrimSpace(candidate.ID)] = candidate
	}
	for _, node := range plan.Source.Nodes {
		nodeByRef[node.NodeRef] = node
	}

	dependencies, err := buildTaskSummaries(plan, task.Deps, taskByID, nodeByRef)
	if err != nil {
		return nil, nil, err
	}

	dependentIDs := make([]string, 0, len(plan.Semantic.Tasks))
	rootID := strings.TrimSpace(task.ID)
	for _, candidate := range plan.Semantic.Tasks {
		candidateID := strings.TrimSpace(candidate.ID)
		if candidateID == "" || candidateID == rootID {
			continue
		}
		for _, depID := range candidate.Deps {
			if strings.TrimSpace(depID) == rootID {
				dependentIDs = append(dependentIDs, candidateID)
				break
			}
		}
	}
	dependents, err := buildTaskSummaries(plan, dependentIDs, taskByID, nodeByRef)
	if err != nil {
		return nil, nil, err
	}
	return dependencies, dependents, nil
}

func buildTaskSummaries(plan ir.PlanIR, ids []string, taskByID map[string]ir.Task, nodeByRef map[string]ir.SourceNode) ([]L2Dependency, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(ids))
	normalized := make([]string, 0, len(ids))
	for _, id := range ids {
		taskID := strings.TrimSpace(id)
		if taskID == "" {
			continue
		}
		if _, exists := seen[taskID]; exists {
			continue
		}
		seen[taskID] = struct{}{}
		normalized = append(normalized, taskID)
	}
	sort.Strings(normalized)

	summaries := make([]L2Dependency, 0, len(normalized))
	for _, id := range normalized {
		task, ok := taskByID[id]
		if !ok {
			return nil, fmt.Errorf("dependency task not found: %s", id)
		}
		node, ok := nodeByRef[task.NodeRef]
		if !ok {
			return nil, fmt.Errorf("source node missing for dependency task %q (node_ref=%s)", task.ID, task.NodeRef)
		}
		summaries = append(summaries, L2Dependency{
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
		})
	}
	return summaries, nil
}

func taskIDsFromSummaries(summaries []L2Dependency) []string {
	if len(summaries) == 0 {
		return nil
	}
	ids := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		if taskID := strings.TrimSpace(summary.TaskID); taskID != "" {
			ids = append(ids, taskID)
		}
	}
	return ids
}

func composeGraphContextClass(hasFiles bool, hasDeps bool, hasDependents bool) string {
	switch {
	case hasFiles && hasDeps && hasDependents:
		return "task+files+deps+dependents"
	case hasFiles && hasDeps:
		return "task+files+deps"
	case hasFiles && hasDependents:
		return "task+files+dependents"
	case hasDeps && hasDependents:
		return "task+deps+dependents"
	case hasFiles:
		return "task+files"
	case hasDeps:
		return "task+deps"
	case hasDependents:
		return "task+dependents"
	default:
		return "task"
	}
}

func directDependenciesHaveOwnDeps(summaries []L2Dependency) bool {
	for _, summary := range summaries {
		if len(summary.Deps) > 0 {
			return true
		}
	}
	return false
}

func estimateTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	runes := len([]rune(text))
	return (runes + 3) / 4
}

func buildIncludedFileRefs(pins []PinExtract) []IncludedFile {
	if len(pins) == 0 {
		return nil
	}
	refs := make([]IncludedFile, 0, len(pins))
	seen := make(map[string]struct{}, len(pins))
	for _, pin := range pins {
		path := strings.TrimSpace(pin.TargetPath)
		if path == "" {
			continue
		}
		reason := reasonForPin(pin)
		key := path + "\x00" + reason
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		refs = append(refs, IncludedFile{
			Path:   path,
			Reason: reason,
		})
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Path != refs[j].Path {
			return refs[i].Path < refs[j].Path
		}
		return refs[i].Reason < refs[j].Reason
	})
	return refs
}

func reasonForPin(pin PinExtract) string {
	switch strings.TrimSpace(pin.Key) {
	case "inferred":
		return "acceptance or scoped task text references repo files"
	default:
		return strings.TrimSpace(pin.Key)
	}
}

func buildIncludedDepRefs(ids []string, reason string) []IncludedDep {
	if len(ids) == 0 {
		return nil
	}
	refs := make([]IncludedDep, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		taskID := strings.TrimSpace(id)
		if taskID == "" {
			continue
		}
		if _, exists := seen[taskID]; exists {
			continue
		}
		seen[taskID] = struct{}{}
		refs = append(refs, IncludedDep{
			TaskID: taskID,
			Reason: reason,
		})
	}
	sort.Slice(refs, func(i, j int) bool {
		return refs[i].TaskID < refs[j].TaskID
	})
	return refs
}

func escalationPath(selected string) []string {
	selected = strings.TrimSpace(selected)
	if selected == "" || selected == "task" {
		return []string{"task"}
	}
	return []string{"task", selected}
}

func lineRangeSize(start int, end int) int {
	if start <= 0 || end < start {
		return 0
	}
	return end - start + 1
}

func readFullPlanStats(plan ir.PlanIR) (string, int) {
	path := strings.TrimSpace(plan.PlanPath)
	if path == "" {
		return "", maxSourceEndLine(plan)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", maxSourceEndLine(plan)
	}
	text := string(payload)
	return text, countTextLines(text)
}

func countTextLines(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}

func maxSourceEndLine(plan ir.PlanIR) int {
	maxEnd := 0
	for _, node := range plan.Source.Nodes {
		if node.EndLine > maxEnd {
			maxEnd = node.EndLine
		}
	}
	return maxEnd
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
