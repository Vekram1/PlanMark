package compile

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type MetadataEntry struct {
	Key    string
	Value  string
	Line   int
	Indent int
}

type MetadataParseResult struct {
	KnownByKey map[string][]MetadataEntry
	Opaque     []MetadataEntry
	All        []MetadataEntry
}

var (
	metadataLinePattern = regexp.MustCompile(`^\s*@([A-Za-z0-9_.-]+)\s*(.*)$`)
	metadataFenceLine   = regexp.MustCompile(`^\s*(` + "```" + `|~~~)`)
)

var knownMetadataKeys = map[string]struct{}{
	"id":        {},
	"horizon":   {},
	"deps":      {},
	"accept":    {},
	"why":       {},
	"touches":   {},
	"non_goal":  {},
	"risk":      {},
	"rollback":  {},
	"assume":    {},
	"invariant": {},
}

func ParseMetadata(content []byte) (MetadataParseResult, error) {
	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(string(content), "\r\n", "\n"), "\r", "\n"), "\n")
	result := MetadataParseResult{
		KnownByKey: make(map[string][]MetadataEntry),
		Opaque:     make([]MetadataEntry, 0),
		All:        make([]MetadataEntry, 0),
	}
	inFence := false

	for idx, line := range lines {
		if metadataFenceLine.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}

		m := metadataLinePattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		key := strings.TrimSpace(strings.ToLower(m[1]))
		if key == "" {
			return MetadataParseResult{}, fmt.Errorf("invalid metadata key at line %d", idx+1)
		}
		entry := MetadataEntry{
			Key:    key,
			Value:  strings.TrimSpace(m[2]),
			Line:   idx + 1,
			Indent: leadingIndentWidth(line),
		}
		result.All = append(result.All, entry)

		if _, ok := knownMetadataKeys[key]; ok {
			result.KnownByKey[key] = append(result.KnownByKey[key], entry)
			continue
		}
		result.Opaque = append(result.Opaque, entry)
	}

	return result, nil
}

func KnownMetadataKeys() []string {
	keys := make([]string, 0, len(knownMetadataKeys))
	for key := range knownMetadataKeys {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
