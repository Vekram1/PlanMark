package change

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"
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
	return LoadPlanGitDiffHintsSince(planPath, "", runner)
}

func LoadPlanGitDiffHintsSince(planPath string, gitRef string, runner GitDiffRunner) ([]GitDiffHunk, error) {
	trimmedPlan := strings.TrimSpace(planPath)
	if trimmedPlan == "" {
		return nil, fmt.Errorf("plan path is required")
	}
	if runner == nil {
		runner = defaultGitDiffRunner
	}
	repoRoot, planRel, err := resolveRepoRootAndPlanPath(trimmedPlan, runner)
	if err != nil {
		return nil, err
	}

	args := []string{"-c", "core.quotepath=false", "-C", repoRoot, "diff", "--unified=0"}
	if strings.TrimSpace(gitRef) != "" {
		args = append(args, gitRef)
	}
	args = append(args, "--", planRel)
	out, err := runner(args...)
	if err != nil {
		return nil, err
	}
	return ParseUnifiedDiffHunks(out), nil
}

func LoadPlanContentAtGitRef(planPath string, gitRef string, runner GitDiffRunner) ([]byte, error) {
	trimmedPlan := strings.TrimSpace(planPath)
	trimmedRef := strings.TrimSpace(gitRef)
	if trimmedPlan == "" {
		return nil, fmt.Errorf("plan path is required")
	}
	if trimmedRef == "" {
		return nil, fmt.Errorf("git ref is required")
	}
	if runner == nil {
		runner = defaultGitDiffRunner
	}
	repoRoot, planRel, err := resolveRepoRootAndPlanPath(trimmedPlan, runner)
	if err != nil {
		return nil, err
	}
	return runner("-c", "core.quotepath=false", "-C", repoRoot, "show", fmt.Sprintf("%s:%s", trimmedRef, planRel))
}

func resolveRepoRootAndPlanPath(planPath string, runner GitDiffRunner) (string, string, error) {
	absPlan, err := filepath.Abs(filepath.Clean(planPath))
	if err != nil {
		return "", "", fmt.Errorf("resolve plan path: %w", err)
	}
	planDir := filepath.Dir(absPlan)
	repoRootRaw, err := runner("-C", planDir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", fmt.Errorf("resolve git repo root: %w", err)
	}
	repoRoot := strings.TrimSpace(string(repoRootRaw))
	if repoRoot == "" {
		return "", "", fmt.Errorf("empty git repo root")
	}
	repoRoot = filepath.Clean(repoRoot)
	if resolvedRoot, err := filepath.EvalSymlinks(repoRoot); err == nil {
		repoRoot = filepath.Clean(resolvedRoot)
	}
	if resolvedPlan, err := filepath.EvalSymlinks(absPlan); err == nil {
		absPlan = filepath.Clean(resolvedPlan)
	}
	rel, err := filepath.Rel(repoRoot, absPlan)
	if err != nil {
		return "", "", fmt.Errorf("resolve repo-relative plan path: %w", err)
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == "." || rel == "" || strings.HasPrefix(rel, "../") || rel == ".." {
		return "", "", fmt.Errorf("plan path %q is outside git repository root %q", planPath, repoRoot)
	}
	return repoRoot, path.Clean(rel), nil
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
	cmd.Env = os.Environ()
	return cmd.Output()
}
