package build

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/ir"
)

const CompileManifestSchemaVersionV01 = "v0.1"

type TaskFingerprintEntry struct {
	ID                  string `json:"id"`
	NodeRef             string `json:"node_ref"`
	SemanticFingerprint string `json:"semantic_fingerprint"`
}

type SourceHashSummary struct {
	NodeCount       int    `json:"node_count"`
	UniqueSliceHash int    `json:"unique_slice_hashes"`
	AggregateHash   string `json:"aggregate_hash"`
}

type OutputHashes struct {
	PlanJSONHash string `json:"plan_json_hash"`
}

type CompileManifest struct {
	SchemaVersion            string                 `json:"schema_version"`
	CompileID                string                 `json:"compile_id"`
	PlanPath                 string                 `json:"plan_path"`
	PlanContentHash          string                 `json:"plan_content_hash"`
	ParserFingerprint        string                 `json:"parser_fingerprint"`
	IRVersion                string                 `json:"ir_version"`
	DeterminismPolicyVersion string                 `json:"determinism_policy_version"`
	SemanticDerivationPolicy string                 `json:"semantic_derivation_policy_version"`
	EffectiveConfigHash      string                 `json:"effective_config_hash"`
	SourceHashSummary        SourceHashSummary      `json:"source_hash_summary"`
	TaskSemanticFingerprints []TaskFingerprintEntry `json:"task_semantic_fingerprints"`
	OutputHashes             OutputHashes           `json:"output_hashes"`
}

type compileIDPayload struct {
	SchemaVersion            string                 `json:"schema_version"`
	PlanPath                 string                 `json:"plan_path"`
	PlanContentHash          string                 `json:"plan_content_hash"`
	ParserFingerprint        string                 `json:"parser_fingerprint"`
	IRVersion                string                 `json:"ir_version"`
	DeterminismPolicyVersion string                 `json:"determinism_policy_version"`
	SemanticDerivationPolicy string                 `json:"semantic_derivation_policy_version"`
	EffectiveConfigHash      string                 `json:"effective_config_hash"`
	SourceHashSummary        SourceHashSummary      `json:"source_hash_summary"`
	TaskFingerprints         []TaskFingerprintEntry `json:"task_semantic_fingerprints"`
	OutputHashes             OutputHashes           `json:"output_hashes"`
}

func DefaultEffectiveConfigHash() string {
	return sha256Hex([]byte("{}"))
}

func BuildCompileManifest(plan ir.PlanIR, planContent []byte, planJSON []byte, effectiveConfigHash string) CompileManifest {
	tasks := make([]TaskFingerprintEntry, 0, len(plan.Semantic.Tasks))
	for _, task := range plan.Semantic.Tasks {
		tasks = append(tasks, TaskFingerprintEntry{
			ID:                  strings.TrimSpace(task.ID),
			NodeRef:             strings.TrimSpace(task.NodeRef),
			SemanticFingerprint: strings.TrimSpace(task.SemanticFingerprint),
		})
	}
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].ID == tasks[j].ID {
			return tasks[i].NodeRef < tasks[j].NodeRef
		}
		return tasks[i].ID < tasks[j].ID
	})

	aggregateParts := make([]string, 0, len(plan.Source.Nodes))
	uniqueSliceHashes := make(map[string]struct{}, len(plan.Source.Nodes))
	for _, node := range plan.Source.Nodes {
		aggregateParts = append(aggregateParts, node.NodeRef+":"+node.SliceHash)
		uniqueSliceHashes[node.SliceHash] = struct{}{}
	}
	sort.Strings(aggregateParts)
	sourceSummary := SourceHashSummary{
		NodeCount:       len(plan.Source.Nodes),
		UniqueSliceHash: len(uniqueSliceHashes),
		AggregateHash:   sha256Hex([]byte(strings.Join(aggregateParts, "\n"))),
	}

	manifest := CompileManifest{
		SchemaVersion:            CompileManifestSchemaVersionV01,
		PlanPath:                 plan.PlanPath,
		PlanContentHash:          sha256Hex(planContent),
		ParserFingerprint:        "parser:v0.1",
		IRVersion:                plan.IRVersion,
		DeterminismPolicyVersion: plan.DeterminismPolicyVersion,
		SemanticDerivationPolicy: plan.SemanticDerivationPolicyVersion,
		EffectiveConfigHash:      strings.TrimSpace(effectiveConfigHash),
		SourceHashSummary:        sourceSummary,
		TaskSemanticFingerprints: tasks,
		OutputHashes: OutputHashes{
			PlanJSONHash: sha256Hex(planJSON),
		},
	}
	if manifest.EffectiveConfigHash == "" {
		manifest.EffectiveConfigHash = DefaultEffectiveConfigHash()
	}

	compileIDBytes, _ := json.Marshal(compileIDPayload{
		SchemaVersion:            manifest.SchemaVersion,
		PlanPath:                 manifest.PlanPath,
		PlanContentHash:          manifest.PlanContentHash,
		ParserFingerprint:        manifest.ParserFingerprint,
		IRVersion:                manifest.IRVersion,
		DeterminismPolicyVersion: manifest.DeterminismPolicyVersion,
		SemanticDerivationPolicy: manifest.SemanticDerivationPolicy,
		EffectiveConfigHash:      manifest.EffectiveConfigHash,
		SourceHashSummary:        manifest.SourceHashSummary,
		TaskFingerprints:         manifest.TaskSemanticFingerprints,
		OutputHashes:             manifest.OutputHashes,
	})
	manifest.CompileID = sha256Hex(compileIDBytes)

	return manifest
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
