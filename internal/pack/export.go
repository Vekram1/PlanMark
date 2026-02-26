package pack

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vikramoddiraju/planmark/internal/build"
	"github.com/vikramoddiraju/planmark/internal/compile"
	contextpkg "github.com/vikramoddiraju/planmark/internal/context"
	"github.com/vikramoddiraju/planmark/internal/ir"
)

const (
	IndexSchemaVersionV01 = "v0.1"
)

type Options struct {
	PlanPath string
	StateDir string
	IDs      []string
	Horizon  string
	Levels   []string
	OutPath  string
}

type Result struct {
	IndexPath string
	Output    string
	PackID    string
	TaskCount int
}

type Index struct {
	SchemaVersion string      `json:"schema_version"`
	PackID        string      `json:"pack_id"`
	PlanPath      string      `json:"plan_path"`
	CompileID     string      `json:"compile_id"`
	Levels        []string    `json:"levels"`
	Tasks         []TaskEntry `json:"tasks"`
	Blobs         []BlobEntry `json:"blobs"`
}

type TaskEntry struct {
	TaskID       string            `json:"task_id"`
	NodeRef      string            `json:"node_ref"`
	PacketPaths  map[string]string `json:"packet_paths"`
	PacketHashes map[string]string `json:"packet_hashes"`
}

type BlobEntry struct {
	Digest string `json:"digest"`
	Path   string `json:"path"`
	Kind   string `json:"kind"`
}

func Export(opts Options) (Result, error) {
	planPath := strings.TrimSpace(opts.PlanPath)
	if planPath == "" {
		return Result{}, fmt.Errorf("plan path is required")
	}
	outPath := strings.TrimSpace(opts.OutPath)
	if outPath == "" {
		return Result{}, fmt.Errorf("output path is required")
	}

	planContent, err := os.ReadFile(planPath)
	if err != nil {
		return Result{}, fmt.Errorf("read plan: %w", err)
	}
	compiled, err := compile.CompilePlan(planPath, planContent, compile.NewParser(nil))
	if err != nil {
		return Result{}, fmt.Errorf("compile plan: %w", err)
	}
	planJSON, err := marshalStable(compiled)
	if err != nil {
		return Result{}, fmt.Errorf("encode plan json: %w", err)
	}

	manifest := build.BuildCompileManifest(compiled, planContent, planJSON, build.DefaultEffectiveConfigHash())
	manifestJSON, err := marshalStable(manifest)
	if err != nil {
		return Result{}, fmt.Errorf("encode compile manifest: %w", err)
	}

	levels, err := normalizeLevels(opts.Levels)
	if err != nil {
		return Result{}, err
	}

	selectedTasks, err := selectTasks(compiled, opts.IDs, opts.Horizon)
	if err != nil {
		return Result{}, err
	}

	packID, err := buildPackID(compiled, manifest, levels, selectedTasks)
	if err != nil {
		return Result{}, err
	}

	tmpRoot, err := os.MkdirTemp("", "planmark-pack-*")
	if err != nil {
		return Result{}, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpRoot)

	packRoot := filepath.Join(tmpRoot, "pack")
	if err := os.MkdirAll(filepath.Join(packRoot, "plan"), 0o755); err != nil {
		return Result{}, fmt.Errorf("create plan dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(packRoot, "packets"), 0o755); err != nil {
		return Result{}, fmt.Errorf("create packets dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(packRoot, "blobs", "sha256"), 0o755); err != nil {
		return Result{}, fmt.Errorf("create blobs dir: %w", err)
	}

	if err := os.WriteFile(filepath.Join(packRoot, "plan", "plan.json"), planJSON, 0o644); err != nil {
		return Result{}, fmt.Errorf("write plan json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(packRoot, "plan", "compile-manifest.json"), manifestJSON, 0o644); err != nil {
		return Result{}, fmt.Errorf("write compile manifest: %w", err)
	}

	blobs := make(map[string]BlobEntry)
	if err := writeBlob(packRoot, blobs, planJSON, "plan_json"); err != nil {
		return Result{}, err
	}
	if err := writeBlob(packRoot, blobs, manifestJSON, "compile_manifest"); err != nil {
		return Result{}, err
	}

	taskEntries := make([]TaskEntry, 0, len(selectedTasks))
	for _, task := range selectedTasks {
		taskPathSegment := safeTaskPathSegment(task.ID)
		taskDir := filepath.Join(packRoot, "packets", taskPathSegment)
		if err := os.MkdirAll(taskDir, 0o755); err != nil {
			return Result{}, fmt.Errorf("create task packets dir: %w", err)
		}
		entry := TaskEntry{
			TaskID:       task.ID,
			NodeRef:      task.NodeRef,
			PacketPaths:  make(map[string]string),
			PacketHashes: make(map[string]string),
		}
		for _, level := range levels {
			packetBytes, err := buildPacketBytes(compiled, task.ID, level)
			if err != nil {
				return Result{}, err
			}
			relPath := filepath.ToSlash(filepath.Join("packets", taskPathSegment, strings.ToLower(level)+".json"))
			absPath := filepath.Join(packRoot, filepath.FromSlash(relPath))
			if err := os.WriteFile(absPath, packetBytes, 0o644); err != nil {
				return Result{}, fmt.Errorf("write packet %s/%s: %w", task.ID, level, err)
			}
			entry.PacketPaths[level] = relPath
			entry.PacketHashes[level] = "sha256:" + sha256Hex(packetBytes)
			if err := writeBlob(packRoot, blobs, packetBytes, "packet"); err != nil {
				return Result{}, err
			}
		}
		taskEntries = append(taskEntries, entry)
	}

	blobEntries := make([]BlobEntry, 0, len(blobs))
	for _, blob := range blobs {
		blobEntries = append(blobEntries, blob)
	}
	sort.Slice(blobEntries, func(i, j int) bool {
		return blobEntries[i].Digest < blobEntries[j].Digest
	})

	index := Index{
		SchemaVersion: IndexSchemaVersionV01,
		PackID:        packID,
		PlanPath:      filepath.ToSlash(planPath),
		CompileID:     manifest.CompileID,
		Levels:        append([]string(nil), levels...),
		Tasks:         taskEntries,
		Blobs:         blobEntries,
	}
	indexBytes, err := marshalStable(index)
	if err != nil {
		return Result{}, fmt.Errorf("encode index: %w", err)
	}
	indexPath := filepath.Join(packRoot, "index.json")
	if err := os.WriteFile(indexPath, indexBytes, 0o644); err != nil {
		return Result{}, fmt.Errorf("write index: %w", err)
	}

	finalIndexPath := ""
	if strings.HasSuffix(strings.ToLower(outPath), ".tar.gz") {
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return Result{}, fmt.Errorf("create output dir: %w", err)
		}
		if err := writeTarGz(outPath, packRoot); err != nil {
			return Result{}, err
		}
		finalIndexPath = "pack/index.json"
	} else {
		packOutputRoot := filepath.Join(outPath, "pack")
		if err := os.RemoveAll(packOutputRoot); err != nil && !os.IsNotExist(err) {
			return Result{}, fmt.Errorf("reset output pack dir: %w", err)
		}
		if err := copyDir(packRoot, packOutputRoot); err != nil {
			return Result{}, err
		}
		finalIndexPath = filepath.ToSlash(filepath.Join(outPath, "pack", "index.json"))
	}

	return Result{
		IndexPath: finalIndexPath,
		Output:    outPath,
		PackID:    packID,
		TaskCount: len(selectedTasks),
	}, nil
}

func selectTasks(plan ir.PlanIR, ids []string, horizon string) ([]ir.Task, error) {
	requested := make([]string, 0, len(ids))
	seen := map[string]struct{}{}
	for _, id := range ids {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		requested = append(requested, trimmed)
	}
	sort.Strings(requested)

	taskByID := make(map[string]ir.Task, len(plan.Semantic.Tasks))
	for _, task := range plan.Semantic.Tasks {
		taskByID[task.ID] = task
	}

	if len(requested) > 0 {
		out := make([]ir.Task, 0, len(requested))
		for _, id := range requested {
			task, ok := taskByID[id]
			if !ok {
				return nil, fmt.Errorf("unknown task id in --ids: %s", id)
			}
			out = append(out, task)
		}
		return out, nil
	}

	trimmedHorizon := strings.TrimSpace(horizon)
	out := make([]ir.Task, 0, len(plan.Semantic.Tasks))
	for _, task := range plan.Semantic.Tasks {
		if trimmedHorizon == "" || strings.EqualFold(strings.TrimSpace(task.Horizon), trimmedHorizon) {
			out = append(out, task)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func normalizeLevels(input []string) ([]string, error) {
	if len(input) == 0 {
		return []string{"L0"}, nil
	}
	order := []string{"L0", "L1", "L2"}
	allowed := map[string]struct{}{
		"L0": {},
		"L1": {},
		"L2": {},
	}
	seen := make(map[string]struct{})
	for _, level := range input {
		normalized := strings.ToUpper(strings.TrimSpace(level))
		if normalized == "" {
			continue
		}
		if _, ok := allowed[normalized]; !ok {
			return nil, fmt.Errorf("invalid level %q", level)
		}
		seen[normalized] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for _, level := range order {
		if _, ok := seen[level]; ok {
			out = append(out, level)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one level is required")
	}
	return out, nil
}

func buildPacketBytes(plan ir.PlanIR, taskID string, level string) ([]byte, error) {
	switch level {
	case "L0":
		packet, err := contextpkg.BuildL0(plan, taskID)
		if err != nil {
			return nil, fmt.Errorf("build packet %s %s: %w", taskID, level, err)
		}
		return marshalStable(packet)
	case "L1":
		packet, err := contextpkg.BuildL1(plan, taskID)
		if err != nil {
			return nil, fmt.Errorf("build packet %s %s: %w", taskID, level, err)
		}
		return marshalStable(packet)
	case "L2":
		return nil, fmt.Errorf("build packet %s %s: not implemented", taskID, level)
	default:
		return nil, fmt.Errorf("invalid level: %s", level)
	}
}

func buildPackID(plan ir.PlanIR, manifest build.CompileManifest, levels []string, tasks []ir.Task) (string, error) {
	taskIDs := make([]string, 0, len(tasks))
	for _, task := range tasks {
		taskIDs = append(taskIDs, task.ID)
	}
	sort.Strings(taskIDs)
	payload := struct {
		PlanPath            string   `json:"plan_path"`
		CompileID           string   `json:"compile_id"`
		Levels              []string `json:"levels"`
		TaskIDs             []string `json:"task_ids"`
		EffectiveConfigHash string   `json:"effective_config_hash"`
	}{
		PlanPath:            filepath.ToSlash(plan.PlanPath),
		CompileID:           manifest.CompileID,
		Levels:              append([]string(nil), levels...),
		TaskIDs:             taskIDs,
		EffectiveConfigHash: manifest.EffectiveConfigHash,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode pack id payload: %w", err)
	}
	return sha256Hex(data), nil
}

func writeBlob(packRoot string, blobs map[string]BlobEntry, data []byte, kind string) error {
	digest := "sha256:" + sha256Hex(data)
	if _, ok := blobs[digest]; ok {
		return nil
	}
	hexDigest := strings.TrimPrefix(digest, "sha256:")
	if len(hexDigest) < 2 {
		return fmt.Errorf("invalid digest %q", digest)
	}
	relPath := filepath.ToSlash(filepath.Join("blobs", "sha256", hexDigest[:2], hexDigest))
	absPath := filepath.Join(packRoot, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return fmt.Errorf("create blob dir: %w", err)
	}
	if err := os.WriteFile(absPath, data, 0o644); err != nil {
		return fmt.Errorf("write blob %s: %w", digest, err)
	}
	blobs[digest] = BlobEntry{
		Digest: digest,
		Path:   relPath,
		Kind:   kind,
	}
	return nil
}

func writeTarGz(outPath string, packRoot string) error {
	outFile, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create tar.gz: %w", err)
	}

	gw, err := gzip.NewWriterLevel(outFile, gzip.BestCompression)
	if err != nil {
		_ = outFile.Close()
		return fmt.Errorf("init gzip writer: %w", err)
	}
	gw.Name = ""
	gw.Comment = ""
	gw.ModTime = time.Unix(0, 0).UTC()

	tw := tar.NewWriter(gw)

	entries := make([]string, 0)
	err = filepath.WalkDir(packRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(packRoot, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		entries = append(entries, rel)
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk pack root: %w", err)
	}
	sort.Strings(entries)

	for _, rel := range entries {
		absPath := filepath.Join(packRoot, rel)
		info, err := os.Lstat(absPath)
		if err != nil {
			return fmt.Errorf("stat %s: %w", rel, err)
		}
		headerName := filepath.ToSlash(filepath.Join("pack", rel))
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("header %s: %w", rel, err)
		}
		header.Name = headerName
		header.ModTime = time.Unix(0, 0).UTC()
		header.AccessTime = time.Unix(0, 0).UTC()
		header.ChangeTime = time.Unix(0, 0).UTC()
		if info.IsDir() {
			header.Mode = 0o755
		} else {
			header.Mode = 0o644
		}
		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("write tar header %s: %w", rel, err)
		}
		if info.IsDir() {
			continue
		}
		file, err := os.Open(absPath)
		if err != nil {
			return fmt.Errorf("open %s: %w", rel, err)
		}
		if _, err := io.Copy(tw, file); err != nil {
			file.Close()
			return fmt.Errorf("copy %s: %w", rel, err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close %s: %w", rel, err)
		}
	}
	if err := tw.Close(); err != nil {
		_ = gw.Close()
		_ = outFile.Close()
		return fmt.Errorf("close tar writer: %w", err)
	}
	if err := gw.Close(); err != nil {
		_ = outFile.Close()
		return fmt.Errorf("close gzip writer: %w", err)
	}
	if err := outFile.Close(); err != nil {
		return fmt.Errorf("close output file: %w", err)
	}
	return nil
}

func copyDir(src string, dst string) error {
	entries := make([]string, 0)
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		entries = append(entries, rel)
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk source dir: %w", err)
	}
	sort.Strings(entries)
	for _, rel := range entries {
		srcPath := filepath.Join(src, rel)
		dstPath := filepath.Join(dst, rel)
		info, err := os.Lstat(srcPath)
		if err != nil {
			return fmt.Errorf("stat %s: %w", rel, err)
		}
		if info.IsDir() {
			if err := os.MkdirAll(dstPath, 0o755); err != nil {
				return fmt.Errorf("create dir %s: %w", rel, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return fmt.Errorf("create parent %s: %w", rel, err)
		}
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", rel, err)
		}
		if err := os.WriteFile(dstPath, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", rel, err)
		}
	}
	return nil
}

func marshalStable(v any) ([]byte, error) {
	payload, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func safeTaskPathSegment(taskID string) string {
	escaped := url.PathEscape(strings.TrimSpace(taskID))
	if escaped == "" {
		return "task"
	}
	return escaped
}
