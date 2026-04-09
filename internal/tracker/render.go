package tracker

import (
	"fmt"
	"strings"
)

type RenderProfile string

const (
	RenderProfileDefault RenderProfile = "default"
	RenderProfileCompact RenderProfile = "compact"
	RenderProfileAgentic RenderProfile = "agentic"
	RenderProfileHandoff RenderProfile = "handoff"
)

type RenderedChecklistItem struct {
	NodeRef string `json:"node_ref,omitempty"`
	Title   string `json:"title"`
	Checked bool   `json:"checked,omitempty"`
}

type RenderedChildWork struct {
	Title string   `json:"title"`
	Body  []string `json:"body,omitempty"`
}

type RenderedTask struct {
	Profile       RenderProfile           `json:"profile"`
	Title         string                  `json:"title"`
	Body          []string                `json:"body,omitempty"`
	Steps         []RenderedChecklistItem `json:"steps,omitempty"`
	StepMode      CapabilitySupport       `json:"step_mode,omitempty"`
	ChildWork     []RenderedChildWork     `json:"child_work,omitempty"`
	ChildWorkMode CapabilitySupport       `json:"child_work_mode,omitempty"`
	CustomFields  map[string]string       `json:"custom_fields,omitempty"`
}

func RenderTask(task TaskProjection, caps TrackerCapabilities, profile RenderProfile) (RenderedTask, error) {
	profile = normalizeRenderProfile(profile)
	title := strings.TrimSpace(task.Title)
	if title == "" {
		return RenderedTask{}, fmt.Errorf("task projection requires non-empty title")
	}
	if !caps.Title {
		return RenderedTask{}, fmt.Errorf("tracker adapter %q does not support titles", caps.AdapterName)
	}

	rendered := RenderedTask{
		Profile: profile,
		Title:   title,
	}

	body := make([]string, 0, 32)
	switch profile {
	case RenderProfileDefault:
		body = append(body, renderSectionBlocks(task.Sections, "## ")...)
		body = append(body, renderDependenciesBlock(task.Dependencies, "## Dependencies")...)
		body = append(body, renderAcceptanceBlock(task.Acceptance, "## Acceptance")...)
		body = append(body, renderEvidenceBlock(task.Evidence, "## Evidence")...)
		body = append(body, renderProvenanceBlock(task, "## Provenance", false)...)
	case RenderProfileCompact:
		body = append(body, renderCompactSections(task.Sections)...)
		body = append(body, renderCompactDependencies(task.Dependencies)...)
		body = append(body, renderCompactAcceptance(task.Acceptance)...)
		body = append(body, renderCompactEvidence(task.Evidence)...)
		body = append(body, renderCompactProvenance(task)...)
	case RenderProfileAgentic:
		body = append(body, renderAgenticHeader(task)...)
		body = append(body, renderSectionBlocks(task.Sections, "## ")...)
		body = append(body, renderDependenciesBlock(task.Dependencies, "## Dependencies")...)
		body = append(body, renderAcceptanceBlock(task.Acceptance, "## Acceptance Targets")...)
		body = append(body, renderEvidenceBlock(task.Evidence, "## Evidence")...)
		body = append(body, renderProvenanceBlock(task, "## Provenance", true)...)
	case RenderProfileHandoff:
		body = append(body, renderAgenticHeader(task)...)
		body = append(body, renderSectionBlocks(task.Sections, "## ")...)
		body = append(body, renderDependenciesBlock(task.Dependencies, "## Dependencies")...)
		body = append(body, renderAcceptanceBlock(task.Acceptance, "## Acceptance Targets")...)
		body = append(body, renderEvidenceBlock(task.Evidence, "## Evidence")...)
		body = append(body, renderProvenanceBlock(task, "## Provenance", true)...)
	default:
		return RenderedTask{}, fmt.Errorf("unsupported render profile %q", profile)
	}

	if len(task.Steps) > 0 {
		switch caps.Steps {
		case CapabilityNative:
			rendered.StepMode = CapabilityNative
			rendered.Steps = renderNativeSteps(task.Steps)
		case CapabilityRendered:
			rendered.StepMode = CapabilityRendered
			body = append(body, renderStepBlock(task.Steps, profileStepHeading(profile))...)
		default:
			return RenderedTask{}, fmt.Errorf("tracker adapter %q does not support rendered or native steps", caps.AdapterName)
		}
	}

	body = trimBlankEdges(body)
	if len(body) > 0 {
		if caps.Body == TextUnsupported {
			return RenderedTask{}, fmt.Errorf("tracker adapter %q does not support body rendering", caps.AdapterName)
		}
		body = normalizeBodyForTextCapability(body, caps.Body)
		rendered.Body = body
	}
	if len(rendered.ChildWork) > 0 && caps.ChildWork == CapabilityUnsupported {
		return RenderedTask{}, fmt.Errorf("tracker adapter %q does not support child work", caps.AdapterName)
	}
	if len(rendered.CustomFields) > 0 && caps.CustomFields == CapabilityUnsupported {
		return RenderedTask{}, fmt.Errorf("tracker adapter %q does not support custom fields", caps.AdapterName)
	}

	return rendered, nil
}

func normalizeRenderProfile(profile RenderProfile) RenderProfile {
	trimmed := strings.TrimSpace(string(profile))
	if trimmed == "" {
		return RenderProfileDefault
	}
	return RenderProfile(strings.ToLower(trimmed))
}

func renderNativeSteps(steps []TaskProjectionStep) []RenderedChecklistItem {
	rendered := make([]RenderedChecklistItem, 0, len(steps))
	for _, step := range steps {
		title := strings.TrimSpace(step.Title)
		if title == "" {
			continue
		}
		rendered = append(rendered, RenderedChecklistItem{
			NodeRef: strings.TrimSpace(step.NodeRef),
			Title:   title,
			Checked: step.Checked,
		})
	}
	return rendered
}

func renderStepBlock(steps []TaskProjectionStep, heading string) []string {
	items := renderNativeSteps(steps)
	if len(items) == 0 {
		return nil
	}
	lines := []string{heading}
	for _, item := range items {
		prefix := "- [ ] "
		if item.Checked {
			prefix = "- [x] "
		}
		lines = append(lines, prefix+item.Title)
	}
	return lines
}

func renderSectionBlocks(sections []TaskProjectionSection, headingPrefix string) []string {
	lines := make([]string, 0, len(sections)*3)
	for _, section := range sections {
		body := normalizedSectionBody(section.Body)
		if len(body) == 0 {
			continue
		}
		title := renderSectionTitle(section)
		if title != "" {
			lines = append(lines, headingPrefix+title)
		}
		lines = append(lines, body...)
	}
	return lines
}

func renderDependenciesBlock(deps []string, heading string) []string {
	deps = normalizedOrderedStrings(deps)
	if len(deps) == 0 {
		return nil
	}
	lines := []string{heading}
	for _, dep := range deps {
		lines = append(lines, "- "+dep)
	}
	return lines
}

func renderAcceptanceBlock(values []string, heading string) []string {
	values = normalizedOrderedStrings(values)
	if len(values) == 0 {
		return nil
	}
	lines := []string{heading}
	for _, value := range values {
		lines = append(lines, "- "+value)
	}
	return lines
}

func renderEvidenceBlock(evidence []TaskProjectionEvidence, heading string) []string {
	evidence = normalizedEvidence(evidence)
	if len(evidence) == 0 {
		return nil
	}
	lines := []string{heading}
	for _, item := range evidence {
		if item.Kind != "" {
			lines = append(lines, "- "+item.Kind+": "+item.NodeRef)
			continue
		}
		lines = append(lines, "- "+item.NodeRef)
	}
	return lines
}

func renderProvenanceBlock(task TaskProjection, heading string, includeCompileID bool) []string {
	provenance := normalizedProvenance(task.Provenance)
	if provenance.Path == "" {
		return nil
	}
	lines := []string{
		heading,
		fmt.Sprintf("- source: %s:%d-%d", provenance.Path, provenance.StartLine, provenance.EndLine),
		fmt.Sprintf("- source_hash: %s", provenance.SourceHash),
	}
	if provenance.NodeRef != "" {
		lines = append(lines, "- node_ref: "+provenance.NodeRef)
	}
	if anchor := strings.TrimSpace(task.Anchor); anchor != "" {
		lines = append(lines, "- anchor: "+anchor)
	}
	if includeCompileID && provenance.CompileID != "" {
		lines = append(lines, "- compile_id: "+provenance.CompileID)
	}
	return lines
}

func renderAgenticHeader(task TaskProjection) []string {
	lines := []string{"## Task"}
	lines = append(lines, "- id: "+strings.TrimSpace(task.ID))
	if horizon := strings.TrimSpace(task.Horizon); horizon != "" {
		lines = append(lines, "- horizon: "+horizon)
	}
	return lines
}

func renderCompactSections(sections []TaskProjectionSection) []string {
	lines := make([]string, 0, len(sections))
	for _, section := range sections {
		body := normalizedSectionBody(section.Body)
		if len(body) == 0 {
			continue
		}
		title := strings.ToLower(renderSectionTitle(section))
		if title == "" {
			title = "notes"
		}
		compactBody := make([]string, 0, len(body))
		for _, line := range body {
			if strings.TrimSpace(line) == "" {
				continue
			}
			compactBody = append(compactBody, line)
		}
		lines = append(lines, title+": "+strings.Join(compactBody, " "))
	}
	return lines
}

func renderCompactDependencies(deps []string) []string {
	deps = normalizedOrderedStrings(deps)
	if len(deps) == 0 {
		return nil
	}
	return []string{"deps: " + strings.Join(deps, ", ")}
}

func renderCompactAcceptance(values []string) []string {
	values = normalizedOrderedStrings(values)
	if len(values) == 0 {
		return nil
	}
	return []string{"accept: " + strings.Join(values, " | ")}
}

func renderCompactEvidence(evidence []TaskProjectionEvidence) []string {
	evidence = normalizedEvidence(evidence)
	if len(evidence) == 0 {
		return nil
	}
	items := make([]string, 0, len(evidence))
	for _, item := range evidence {
		label := item.NodeRef
		if item.Kind != "" {
			label = item.Kind + ":" + item.NodeRef
		}
		items = append(items, label)
	}
	return []string{"evidence: " + strings.Join(items, ", ")}
}

func renderCompactProvenance(task TaskProjection) []string {
	provenance := normalizedProvenance(task.Provenance)
	if provenance.Path == "" {
		return nil
	}
	return []string{fmt.Sprintf("provenance: %s:%d-%d", provenance.Path, provenance.StartLine, provenance.EndLine)}
}

func renderSectionTitle(section TaskProjectionSection) string {
	if title := strings.TrimSpace(section.Title); title != "" {
		return title
	}
	if key := strings.TrimSpace(section.Key); key != "" {
		return strings.ReplaceAll(strings.Title(strings.ReplaceAll(key, "_", " ")), "  ", " ")
	}
	return ""
}

func trimBlankEdges(lines []string) []string {
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return append([]string(nil), lines[start:end]...)
}

func normalizeBodyForTextCapability(lines []string, capability TextCapability) []string {
	if capability != TextPlain {
		return lines
	}
	normalized := make([]string, 0, len(lines))
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "## "):
			normalized = append(normalized, strings.TrimSpace(strings.TrimPrefix(line, "## "))+":")
		case strings.HasPrefix(line, "- [ ] "):
			normalized = append(normalized, "[ ] "+strings.TrimSpace(strings.TrimPrefix(line, "- [ ] ")))
		case strings.HasPrefix(line, "- [x] "):
			normalized = append(normalized, "[x] "+strings.TrimSpace(strings.TrimPrefix(line, "- [x] ")))
		default:
			normalized = append(normalized, line)
		}
	}
	return normalized
}

func profileStepHeading(profile RenderProfile) string {
	switch profile {
	case RenderProfileCompact:
		return "steps"
	default:
		return "## Steps"
	}
}
