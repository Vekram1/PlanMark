package build

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/fsio"
)

func WriteCompileManifest(stateDir string, manifest CompileManifest) (string, error) {
	resolved := strings.TrimSpace(stateDir)
	if resolved == "" {
		return "", fmt.Errorf("state directory is empty")
	}
	manifestPath := filepath.Join(resolved, "build", "compile-manifest.json")

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal compile manifest: %w", err)
	}
	data = append(data, '\n')
	if err := fsio.WriteFileAtomic(manifestPath, data, 0o644); err != nil {
		return "", fmt.Errorf("write compile manifest: %w", err)
	}
	return manifestPath, nil
}

func ReadCompileManifest(stateDir string) (CompileManifest, error) {
	manifestPath, err := CompileManifestPath(stateDir)
	if err != nil {
		return CompileManifest{}, err
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return CompileManifest{}, err
	}
	var manifest CompileManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return CompileManifest{}, fmt.Errorf("unmarshal compile manifest: %w", err)
	}
	return manifest, nil
}

func CompileManifestPath(stateDir string) (string, error) {
	resolved := strings.TrimSpace(stateDir)
	if resolved == "" {
		return "", fmt.Errorf("state directory is empty")
	}
	return filepath.Join(resolved, "build", "compile-manifest.json"), nil
}
