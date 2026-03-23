package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const fileName = ".planmark.yaml"

type Resolved struct {
	Found   bool
	Path    string
	Profile string
	Hash    string
	Tracker TrackerResolved
	AI      AIResolved
}

type TrackerResolved struct {
	Adapter string
	Profile string
}

type AIResolved struct {
	Provider   string
	Model      string
	BaseURL    string
	APIKeyEnv  string
	TimeoutSec string
}

type rawConfig struct {
	SchemaVersion string
	Profile       string
	Profiles      map[string]string
	Policies      map[string]string
	Tracker       map[string]string
	AI            map[string]string
}

type hashKV struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type hashPayload struct {
	SchemaVersion string   `json:"schema_version,omitempty"`
	Profile       string   `json:"profile,omitempty"`
	Profiles      []hashKV `json:"profiles,omitempty"`
	Policies      []hashKV `json:"policies,omitempty"`
	Tracker       []hashKV `json:"tracker,omitempty"`
	AI            []hashKV `json:"ai,omitempty"`
}

func LoadForPlan(planPath string) (Resolved, error) {
	cfgPath, err := findConfigPath(planPath)
	if err != nil {
		return Resolved{}, err
	}
	if cfgPath == "" {
		return Resolved{Found: false}, nil
	}

	content, err := os.ReadFile(cfgPath)
	if err != nil {
		return Resolved{}, fmt.Errorf("read %s: %w", fileName, err)
	}

	cfg, err := parseYAML(content)
	if err != nil {
		return Resolved{}, fmt.Errorf("parse %s: %w", fileName, err)
	}

	profile := strings.TrimSpace(cfg.Profile)
	if profile == "" {
		profile = strings.TrimSpace(cfg.Profiles["doctor"])
	}
	ai, err := resolveAI(cfg.AI)
	if err != nil {
		return Resolved{}, err
	}
	trackerResolved, err := resolveTracker(cfg.Tracker)
	if err != nil {
		return Resolved{}, err
	}

	return Resolved{
		Found:   true,
		Path:    cfgPath,
		Profile: profile,
		Hash:    canonicalHash(cfg),
		Tracker: trackerResolved,
		AI:      ai,
	}, nil
}

func findConfigPath(planPath string) (string, error) {
	absPlan, err := filepath.Abs(planPath)
	if err != nil {
		return "", fmt.Errorf("resolve plan path: %w", err)
	}
	dir := filepath.Dir(absPlan)
	for {
		candidate := filepath.Join(dir, fileName)
		info, err := os.Stat(candidate)
		if err == nil {
			if info.IsDir() {
				return "", fmt.Errorf("%s is a directory", candidate)
			}
			return candidate, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("stat %s: %w", candidate, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", nil
}

func parseYAML(content []byte) (rawConfig, error) {
	cfg := rawConfig{
		Profiles: make(map[string]string),
		Policies: make(map[string]string),
		Tracker:  make(map[string]string),
		AI:       make(map[string]string),
	}
	lines := strings.Split(string(content), "\n")
	section := ""
	for i, line := range lines {
		lineNumber := i + 1
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if strings.Contains(trimmed, "\t") {
			return rawConfig{}, fmt.Errorf("line %d: tabs are not supported", lineNumber)
		}

		key, value, hasColon := splitKeyValue(trimmed)
		if !hasColon {
			return rawConfig{}, fmt.Errorf("line %d: expected key: value", lineNumber)
		}

		if indent == 0 {
			section = ""
			switch key {
			case "schema_version", "version":
				cfg.SchemaVersion = value
			case "profile":
				cfg.Profile = value
			case "profiles", "policies", "tracker", "ai":
				if value != "" {
					return rawConfig{}, fmt.Errorf("line %d: %s must be a mapping", lineNumber, key)
				}
				section = key
			default:
				return rawConfig{}, fmt.Errorf("line %d: unknown top-level key %q", lineNumber, key)
			}
			continue
		}

		if indent < 2 {
			return rawConfig{}, fmt.Errorf("line %d: nested keys must be indented by at least two spaces", lineNumber)
		}
		if section == "" {
			return rawConfig{}, fmt.Errorf("line %d: nested key without parent section", lineNumber)
		}
		if key == "" || value == "" {
			return rawConfig{}, fmt.Errorf("line %d: nested entry must be key: value", lineNumber)
		}

		switch section {
		case "profiles":
			cfg.Profiles[key] = value
		case "policies":
			cfg.Policies[key] = value
		case "tracker":
			cfg.Tracker[key] = value
		case "ai":
			cfg.AI[key] = value
		default:
			return rawConfig{}, fmt.Errorf("line %d: unsupported section %q", lineNumber, section)
		}
	}

	return cfg, nil
}

func splitKeyValue(line string) (key string, value string, ok bool) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	key = strings.TrimSpace(parts[0])
	value = strings.TrimSpace(parts[1])
	if strings.Contains(value, "#") {
		idx := strings.Index(value, "#")
		if idx == 0 {
			value = ""
		} else if value[idx-1] == ' ' {
			value = strings.TrimSpace(value[:idx])
		}
	}
	return key, value, true
}

func canonicalHash(cfg rawConfig) string {
	payload := hashPayload{
		SchemaVersion: strings.TrimSpace(cfg.SchemaVersion),
		Profile:       strings.TrimSpace(cfg.Profile),
		Profiles:      sortedMap(cfg.Profiles),
		Policies:      sortedMap(cfg.Policies),
		Tracker:       sortedMap(cfg.Tracker),
		AI:            sortedMap(cfg.AI),
	}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func sortedMap(m map[string]string) []hashKV {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, strings.TrimSpace(key))
	}
	sort.Strings(keys)
	out := make([]hashKV, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, hashKV{
			Key:   key,
			Value: strings.TrimSpace(m[key]),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func resolveAI(entries map[string]string) (AIResolved, error) {
	allowed := map[string]struct{}{
		"provider":        {},
		"model":           {},
		"base_url":        {},
		"api_key_env":     {},
		"timeout_seconds": {},
	}
	for key := range entries {
		trimmed := strings.TrimSpace(key)
		if _, ok := allowed[trimmed]; !ok {
			return AIResolved{}, fmt.Errorf("parse %s: unknown ai key %q", fileName, trimmed)
		}
	}
	if raw := strings.TrimSpace(entries["timeout_seconds"]); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return AIResolved{}, fmt.Errorf("parse %s: ai.timeout_seconds must be a positive integer", fileName)
		}
	}
	return AIResolved{
		Provider:   strings.TrimSpace(entries["provider"]),
		Model:      strings.TrimSpace(entries["model"]),
		BaseURL:    strings.TrimSpace(entries["base_url"]),
		APIKeyEnv:  strings.TrimSpace(entries["api_key_env"]),
		TimeoutSec: strings.TrimSpace(entries["timeout_seconds"]),
	}, nil
}

func resolveTracker(entries map[string]string) (TrackerResolved, error) {
	allowed := map[string]struct{}{
		"adapter": {},
		"profile": {},
	}
	for key := range entries {
		trimmed := strings.TrimSpace(key)
		if _, ok := allowed[trimmed]; !ok {
			return TrackerResolved{}, fmt.Errorf("parse %s: unknown tracker key %q", fileName, trimmed)
		}
	}
	return TrackerResolved{
		Adapter: strings.TrimSpace(entries["adapter"]),
		Profile: strings.TrimSpace(entries["profile"]),
	}, nil
}
