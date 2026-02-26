package context

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/ir"
)

var pinTargetPattern = regexp.MustCompile(`^(.+?)(?::(\d+)(?:-(\d+))?)?$`)
var metadataFenceLine = regexp.MustCompile(`^\s*(` + "```" + `|~~~)`)

func extractPinTargets(plan ir.PlanIR, taskNode ir.SourceNode) ([]PinExtract, error) {
	content, err := os.ReadFile(plan.PlanPath)
	if err != nil {
		return nil, fmt.Errorf("read plan for pin extraction: %w", err)
	}

	lines := normalizedLines(string(content))
	nodeLineStarts := make([]int, 0, len(plan.Source.Nodes))
	for _, n := range plan.Source.Nodes {
		nodeLineStarts = append(nodeLineStarts, n.Line)
	}

	nextNodeLine := len(lines) + 1
	for _, line := range nodeLineStarts {
		if line > taskNode.Line && line < nextNodeLine {
			nextNodeLine = line
		}
	}

	repoRoot := filepath.Dir(plan.PlanPath)
	metadata := make([]PinExtract, 0)
	inFence := false
	for line := taskNode.Line + 1; line < nextNodeLine && line <= len(lines); line++ {
		rawLine := lines[line-1]
		if metadataFenceLine.MatchString(rawLine) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		raw := strings.TrimSpace(rawLine)
		if !strings.HasPrefix(raw, "@") {
			continue
		}
		fields := strings.Fields(raw)
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimPrefix(strings.ToLower(fields[0]), "@")
		if key != "pin" && key != "touches" {
			continue
		}

		ref := strings.TrimSpace(strings.TrimPrefix(raw, fields[0]))
		pin, err := buildPinExtract(repoRoot, key, ref)
		if err != nil {
			return nil, err
		}
		metadata = append(metadata, pin)
	}
	return metadata, nil
}

func buildPinExtract(repoRoot string, key string, ref string) (PinExtract, error) {
	targetPath, startLine, endLine, err := parsePinTarget(ref)
	if err != nil {
		return PinExtract{}, fmt.Errorf("parse @%s target %q: %w", key, ref, err)
	}

	cleanPath, absPath, err := resolveRepoScopedPath(repoRoot, targetPath)
	if err != nil {
		return PinExtract{}, fmt.Errorf("@%s target %q: %w", key, targetPath, err)
	}
	content, err := os.ReadFile(absPath)
	if err != nil {
		return PinExtract{}, fmt.Errorf("read @%s target %q: %w", key, cleanPath, err)
	}

	lines := normalizedLines(string(content))
	if len(lines) == 0 {
		return PinExtract{}, fmt.Errorf("@%s target %q has no content", key, cleanPath)
	}
	if startLine < 1 {
		startLine = 1
	}
	if endLine == 0 {
		endLine = len(lines)
	}
	if startLine > len(lines) {
		startLine = len(lines)
	}
	if endLine < startLine {
		endLine = startLine
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}

	slice := strings.Join(lines[startLine-1:endLine], "\n")
	sum := sha256.Sum256([]byte(slice))
	return PinExtract{
		Key:        key,
		TargetPath: filepath.ToSlash(cleanPath),
		StartLine:  startLine,
		EndLine:    endLine,
		TargetHash: hex.EncodeToString(sum[:]),
		SliceText:  slice,
	}, nil
}

func parsePinTarget(ref string) (string, int, int, error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return "", 0, 0, fmt.Errorf("empty target")
	}

	m := pinTargetPattern.FindStringSubmatch(trimmed)
	if m == nil {
		return "", 0, 0, fmt.Errorf("invalid target syntax")
	}

	startLine := 1
	endLine := 0
	if m[2] != "" {
		n, err := strconv.Atoi(m[2])
		if err != nil {
			return "", 0, 0, fmt.Errorf("invalid start line")
		}
		startLine = n
		endLine = n
	}
	if m[3] != "" {
		n, err := strconv.Atoi(m[3])
		if err != nil {
			return "", 0, 0, fmt.Errorf("invalid end line")
		}
		endLine = n
	}
	if endLine != 0 && endLine < startLine {
		return "", 0, 0, fmt.Errorf("invalid range: end line before start line")
	}

	return m[1], startLine, endLine, nil
}

func normalizedLines(content string) []string {
	if content == "" {
		return []string{}
	}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	return strings.Split(content, "\n")
}
