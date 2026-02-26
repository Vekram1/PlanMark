package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/fsio"
)

const (
	ContextKeySchemaVersionV01      = "v0.1"
	CompileReuseKeySchemaVersionV01 = "v0.1"
	contextPacketCacheDirectory     = "context"
	contextPacketCacheFileExtension = ".json"
)

var cacheKeyPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type ContextKeyInput struct {
	SchemaVersion                   string   `json:"schema_version"`
	Level                           string   `json:"level"`
	PlanPath                        string   `json:"plan_path"`
	IRVersion                       string   `json:"ir_version"`
	DeterminismPolicyVersion        string   `json:"determinism_policy_version"`
	SemanticDerivationPolicyVersion string   `json:"semantic_derivation_policy_version"`
	TaskID                          string   `json:"task_id"`
	TaskNodeRef                     string   `json:"task_node_ref"`
	TaskSemanticFingerprint         string   `json:"task_semantic_fingerprint"`
	NodeSliceHash                   string   `json:"node_slice_hash"`
	PinTargetHashes                 []string `json:"pin_target_hashes,omitempty"`
}

type CompileReuseInput struct {
	SchemaVersion                   string `json:"schema_version"`
	PlanPath                        string `json:"plan_path"`
	PlanContentHash                 string `json:"plan_content_hash"`
	ParserFingerprint               string `json:"parser_fingerprint"`
	IRVersion                       string `json:"ir_version"`
	DeterminismPolicyVersion        string `json:"determinism_policy_version"`
	SemanticDerivationPolicyVersion string `json:"semantic_derivation_policy_version"`
	EffectiveConfigHash             string `json:"effective_config_hash"`
}

func ContextPacketKey(input ContextKeyInput) string {
	schemaVersion := strings.TrimSpace(input.SchemaVersion)
	if schemaVersion == "" {
		schemaVersion = ContextKeySchemaVersionV01
	}

	pinHashes := append([]string(nil), input.PinTargetHashes...)
	sort.Strings(pinHashes)

	canonical := ContextKeyInput{
		SchemaVersion:                   schemaVersion,
		Level:                           strings.ToUpper(strings.TrimSpace(input.Level)),
		PlanPath:                        filepath.ToSlash(strings.TrimSpace(input.PlanPath)),
		IRVersion:                       strings.TrimSpace(input.IRVersion),
		DeterminismPolicyVersion:        strings.TrimSpace(input.DeterminismPolicyVersion),
		SemanticDerivationPolicyVersion: strings.TrimSpace(input.SemanticDerivationPolicyVersion),
		TaskID:                          strings.TrimSpace(input.TaskID),
		TaskNodeRef:                     strings.TrimSpace(input.TaskNodeRef),
		TaskSemanticFingerprint:         strings.TrimSpace(input.TaskSemanticFingerprint),
		NodeSliceHash:                   strings.TrimSpace(input.NodeSliceHash),
		PinTargetHashes:                 pinHashes,
	}

	payload, _ := json.Marshal(canonical)
	return sha256Hex(payload)
}

func CompileReuseKey(input CompileReuseInput) string {
	schemaVersion := strings.TrimSpace(input.SchemaVersion)
	if schemaVersion == "" {
		schemaVersion = CompileReuseKeySchemaVersionV01
	}

	canonical := CompileReuseInput{
		SchemaVersion:                   schemaVersion,
		PlanPath:                        filepath.ToSlash(strings.TrimSpace(input.PlanPath)),
		PlanContentHash:                 strings.TrimSpace(input.PlanContentHash),
		ParserFingerprint:               strings.TrimSpace(input.ParserFingerprint),
		IRVersion:                       strings.TrimSpace(input.IRVersion),
		DeterminismPolicyVersion:        strings.TrimSpace(input.DeterminismPolicyVersion),
		SemanticDerivationPolicyVersion: strings.TrimSpace(input.SemanticDerivationPolicyVersion),
		EffectiveConfigHash:             strings.TrimSpace(input.EffectiveConfigHash),
	}

	payload, _ := json.Marshal(canonical)
	return sha256Hex(payload)
}

func ReadContextPacket(stateDir string, key string) ([]byte, error) {
	cachePath, err := contextPacketPath(stateDir, key)
	if err != nil {
		return nil, err
	}
	payload, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func WriteContextPacket(stateDir string, key string, payload []byte) (string, error) {
	cachePath, err := contextPacketPath(stateDir, key)
	if err != nil {
		return "", err
	}
	if err := fsio.WriteFileAtomic(cachePath, payload, 0o644); err != nil {
		return "", fmt.Errorf("write context cache packet: %w", err)
	}
	return cachePath, nil
}

func contextPacketPath(stateDir string, key string) (string, error) {
	resolvedStateDir := strings.TrimSpace(stateDir)
	if resolvedStateDir == "" {
		return "", fmt.Errorf("state directory must not be empty")
	}
	if !cacheKeyPattern.MatchString(key) {
		return "", fmt.Errorf("invalid cache key %q", key)
	}

	return filepath.Join(resolvedStateDir, "cache", contextPacketCacheDirectory, key+contextPacketCacheFileExtension), nil
}

func sha256Hex(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
