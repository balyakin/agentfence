package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/policy"
	"github.com/agentfence/agentfence/internal/ports"
)

type Manager struct {
	blobReader ports.GitRunner
}

var _ ports.WorkspaceManager = (*Manager)(nil)

func NewManager(blobReader ports.GitRunner) *Manager {
	return &Manager{blobReader: blobReader}
}

func (m *Manager) Create(ctx context.Context, req domain.CreateWorkspaceRequest) (domain.WorkspaceResult, error) {
	result := domain.WorkspaceResult{
		RunDir:       req.RunDir,
		ShadowPath:   filepath.Join(req.RunDir, "shadow"),
		ScannerDir:   filepath.Join(req.RunDir, "scanner"),
		LogsDir:      filepath.Join(req.RunDir, "logs"),
		MetadataPath: filepath.Join(req.RunDir, "shadow_metadata.json"),
		PatchPath:    filepath.Join(req.RunDir, "changes.patch"),
	}
	for _, dir := range []string{result.RunDir, result.ShadowPath, result.ScannerDir, result.LogsDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return domain.WorkspaceResult{}, fmt.Errorf("create workspace directory: %w", err)
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return domain.WorkspaceResult{}, fmt.Errorf("chmod workspace directory: %w", err)
		}
	}
	for _, file := range req.ExposurePlan.Files {
		if err := m.copyFile(ctx, req.RepoRoot, result.ShadowPath, file); err != nil {
			return domain.WorkspaceResult{}, err
		}
	}
	if err := initBaseline(ctx, result.ShadowPath); err != nil {
		return domain.WorkspaceResult{}, err
	}
	envResult, err := buildSanitizedEnv(req.RepoRoot, result.ShadowPath, req.SanitizedEnv)
	if err != nil {
		return domain.WorkspaceResult{}, err
	}
	result.SanitizedEnv = envResult.Env
	result.SanitizedEnvKeys = envResult.Keys
	result.SanitizedEnvSources = envResult.SourceFiles
	if envResult.IgnoredPatchPath != "" {
		result.IgnoredPatchPaths = append(result.IgnoredPatchPaths, envResult.IgnoredPatchPath)
	}
	if err := writeMetadata(result.MetadataPath, req, envResult, time.Now().UTC()); err != nil {
		return domain.WorkspaceResult{}, err
	}
	return result, nil
}

func (m *Manager) copyFile(ctx context.Context, repoRoot string, shadowRoot string, file domain.ExposureFile) error {
	if strings.EqualFold(file.RelativePath, ".gitmodules") {
		return domain.ErrUnsafePath
	}
	target, err := policy.SafeJoin(shadowRoot, file.RelativePath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return fmt.Errorf("create shadow parent: %w", err)
	}
	mode := file.Mode.Perm() & 0o755
	if mode == 0 {
		mode = 0o644
	}
	if file.Source == "head" {
		data, blobMode, err := m.blobReader.ReadBlob(ctx, repoRoot, file.RelativePath)
		if err != nil {
			return fmt.Errorf("read tracked blob: %w", err)
		}
		if blobMode.Perm()&0o111 != 0 {
			mode = blobMode.Perm() & 0o755
		}
		if err := os.WriteFile(target, data, mode); err != nil {
			return fmt.Errorf("write shadow blob: %w", err)
		}
		if err := os.Chmod(target, mode); err != nil {
			return fmt.Errorf("chmod shadow blob: %w", err)
		}
		return nil
	}
	source, err := policy.SafeJoin(repoRoot, file.RelativePath)
	if err != nil {
		return err
	}
	if err := policy.ValidateSymlinkInside(repoRoot, source); err != nil {
		return err
	}
	mode = mode & 0o755
	if mode == 0 {
		mode = 0o644
	}
	in, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open worktree file: %w", err)
	}
	inClosed := false
	defer func() {
		if !inClosed {
			_ = in.Close()
		}
	}()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("create shadow file: %w", err)
	}
	outClosed := false
	defer func() {
		if !outClosed {
			_ = out.Close()
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy shadow file: %w", err)
	}
	if err := in.Close(); err != nil {
		return fmt.Errorf("close worktree file: %w", err)
	}
	inClosed = true
	if err := out.Close(); err != nil {
		return fmt.Errorf("close shadow file: %w", err)
	}
	outClosed = true
	return nil
}

func (m *Manager) ValidatePostRunWorkspace(ctx context.Context, shadowPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	root := filepath.Clean(shadowPath)
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		name := entry.Name()
		if strings.EqualFold(name, ".git") {
			if filepath.Dir(path) != root || !entry.IsDir() {
				return domain.ErrPostScanBlocked
			}
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat postrun entry: %w", err)
		}
		mode := info.Mode()
		if mode&os.ModeSymlink != 0 {
			if err := policy.ValidateSymlinkInside(root, path); err != nil {
				return domain.ErrPostScanBlocked
			}
		}
		if mode&(os.ModeSocket|os.ModeDevice|os.ModeNamedPipe) != 0 {
			return domain.ErrPostScanBlocked
		}
		return nil
	})
}

type metadata struct {
	RepoPath        string               `json:"repo_path"`
	RepoHead        string               `json:"repo_head"`
	BaseRef         string               `json:"base_ref"`
	CreatedAt       string               `json:"created_at"`
	ExposureSummary exposureSummary      `json:"exposure_summary"`
	SkippedFiles    []domain.SkippedFile `json:"skipped_files"`
	SanitizedEnv    sanitizedEnvMetadata `json:"sanitized_env"`
}

type exposureSummary struct {
	FileCount int   `json:"file_count"`
	TotalSize int64 `json:"total_size"`
}

type sanitizedEnvMetadata struct {
	Enabled     bool     `json:"enabled"`
	Keys        []string `json:"keys"`
	SourceFiles []string `json:"source_files"`
}

func writeMetadata(path string, req domain.CreateWorkspaceRequest, envResult sanitizedEnvResult, now time.Time) error {
	payload := metadata{
		RepoPath:  req.RepoRoot,
		RepoHead:  req.RepoHead,
		BaseRef:   req.BaseRef,
		CreatedAt: now.Format(time.RFC3339Nano),
		ExposureSummary: exposureSummary{
			FileCount: len(req.ExposurePlan.Files),
			TotalSize: req.ExposurePlan.TotalSize,
		},
		SkippedFiles: req.ExposurePlan.SkippedFiles,
		SanitizedEnv: sanitizedEnvMetadata{
			Enabled:     req.SanitizedEnv.Enabled,
			Keys:        envResult.Keys,
			SourceFiles: envResult.SourceFiles,
		},
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal shadow metadata: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write shadow metadata: %w", err)
	}
	return nil
}

func runGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return nil, fmt.Errorf("git %s failed: %w: %s", args[0], err, detail)
		}
		return nil, fmt.Errorf("git %s failed: %w", args[0], err)
	}
	return output, nil
}
