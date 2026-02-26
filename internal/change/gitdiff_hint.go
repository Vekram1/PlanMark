package change

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type GitDiffHunk struct {
	Path      string `json:"path"`
	NewStart  int    `json:"new_start"`
	NewLength int    `json:"new_length"`
}

type GitDiffRunner func(args ...string) ([]byte, error)

func LoadPlanGitDiffHints(planPath string, runner GitDiffRunner) ([]GitDiffHunk, error) {
	trimmedPlan := strings.TrimSpace(planPath)
	if trimmedPlan == "" {
		return nil, fmt.Errorf("plan path is required")
	}
	if runner == nil {
		runner = defaultGitDiffRunner
	}

	out, err := runner("-c", "core.quotepath=false", "diff", "--unified=0", "--", filepath.Clean(trimmedPlan))
	if err != nil {
		return nil, err
	}
	return ParseUnifiedDiffHunks(out), nil
}

func ParseUnifiedDiffHunks(diff []byte) []GitDiffHunk {
	lines := bytes.Split(diff, []byte{'\n'})
	currentPath := ""
	hunks := make([]GitDiffHunk, 0)

	for _, line := range lines {
		text := string(line)
		if strings.HasPrefix(text, "+++ ") {
			path := strings.TrimSpace(strings.TrimPrefix(text, "+++ "))
			if strings.HasPrefix(path, "b/") {
				path = strings.TrimPrefix(path, "b/")
			}
			currentPath = path
			continue
		}
		if !strings.HasPrefix(text, "@@ ") {
			continue
		}
		newStart, newLen, ok := parseUnifiedNewRange(text)
		if !ok || currentPath == "" {
			continue
		}
		hunks = append(hunks, GitDiffHunk{
			Path:      currentPath,
			NewStart:  newStart,
			NewLength: newLen,
		})
	}

	sort.Slice(hunks, func(i, j int) bool {
		if hunks[i].Path != hunks[j].Path {
			return hunks[i].Path < hunks[j].Path
		}
		if hunks[i].NewStart != hunks[j].NewStart {
			return hunks[i].NewStart < hunks[j].NewStart
		}
		return hunks[i].NewLength < hunks[j].NewLength
	})
	return hunks
}

func parseUnifiedNewRange(header string) (int, int, bool) {
	parts := strings.Split(header, " ")
	if len(parts) < 3 {
		return 0, 0, false
	}
	rangeToken := strings.TrimPrefix(parts[2], "+")
	if rangeToken == "" {
		return 0, 0, false
	}
	segment := strings.Split(rangeToken, ",")
	start, err := strconv.Atoi(segment[0])
	if err != nil {
		return 0, 0, false
	}
	length := 1
	if len(segment) > 1 {
		length, err = strconv.Atoi(segment[1])
		if err != nil {
			return 0, 0, false
		}
	}
	return start, length, true
}

func defaultGitDiffRunner(args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	return cmd.Output()
}
