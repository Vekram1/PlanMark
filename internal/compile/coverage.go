package compile

import (
	"fmt"
	"sort"
)

type LineRange struct {
	StartLine int `json:"start_line"`
	EndLine   int `json:"end_line"`
}

type SourceCoverage struct {
	Interpreted []LineRange `json:"interpreted"`
	Opaque      []LineRange `json:"opaque"`
	TotalLines  int         `json:"total_lines"`
	Unaccounted int         `json:"unaccounted_lines"`
}

func ComputeSourceCoverage(content []byte, compiled []CompiledNode) (SourceCoverage, error) {
	lines := normalizedLines(string(content))
	total := len(lines)
	if total == 0 {
		return SourceCoverage{}, nil
	}

	covered := make([]bool, total+1) // 1-based line indexing
	interpreted := make([]LineRange, 0, len(compiled))
	for _, cn := range compiled {
		start, end := cn.Slice.StartLine, cn.Slice.EndLine
		if start <= 0 || end < start || end > total {
			return SourceCoverage{}, fmt.Errorf("invalid compiled range %d-%d", start, end)
		}
		interpreted = append(interpreted, LineRange{StartLine: start, EndLine: end})
		for line := start; line <= end; line++ {
			covered[line] = true
		}
	}
	interpreted = mergeRanges(interpreted)

	opaque := make([]LineRange, 0)
	line := 1
	for line <= total {
		if covered[line] {
			line++
			continue
		}
		start := line
		for line <= total && !covered[line] {
			line++
		}
		opaque = append(opaque, LineRange{StartLine: start, EndLine: line - 1})
	}

	accounted := 0
	for _, r := range interpreted {
		accounted += r.EndLine - r.StartLine + 1
	}
	for _, r := range opaque {
		accounted += r.EndLine - r.StartLine + 1
	}

	unaccounted := total - accounted
	if unaccounted < 0 {
		unaccounted = 0
	}

	return SourceCoverage{
		Interpreted: interpreted,
		Opaque:      opaque,
		TotalLines:  total,
		Unaccounted: unaccounted,
	}, nil
}

func mergeRanges(in []LineRange) []LineRange {
	if len(in) <= 1 {
		return append([]LineRange(nil), in...)
	}
	cp := append([]LineRange(nil), in...)
	sort.Slice(cp, func(i, j int) bool {
		if cp[i].StartLine != cp[j].StartLine {
			return cp[i].StartLine < cp[j].StartLine
		}
		return cp[i].EndLine < cp[j].EndLine
	})

	merged := make([]LineRange, 0, len(cp))
	curr := cp[0]
	for _, r := range cp[1:] {
		if r.StartLine <= curr.EndLine+1 {
			if r.EndLine > curr.EndLine {
				curr.EndLine = r.EndLine
			}
			continue
		}
		merged = append(merged, curr)
		curr = r
	}
	merged = append(merged, curr)
	return merged
}
