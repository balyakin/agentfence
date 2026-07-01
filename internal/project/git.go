package project

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/ports"
)

type Git struct {
	executable string
}

var _ ports.GitRunner = (*Git)(nil)

func NewGit() *Git {
	return &Git{executable: "git"}
}

func (g *Git) FindRepoRoot(ctx context.Context, startDir string) (string, error) {
	output, err := g.run(ctx, startDir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", domain.ErrNotGitRepo
	}
	root := strings.TrimSpace(string(output))
	if root == "" {
		return "", domain.ErrNotGitRepo
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("absolute repo root: %w", err)
	}
	return abs, nil
}

func (g *Git) HeadSHA(ctx context.Context, repoRoot string) (string, error) {
	output, err := g.run(ctx, repoRoot, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git head sha: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func (g *Git) BaseRef(ctx context.Context, repoRoot string) (string, error) {
	output, err := g.run(ctx, repoRoot, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git base ref: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func (g *Git) StatusPorcelain(ctx context.Context, repoRoot string) ([]domain.StatusEntry, error) {
	output, err := g.run(ctx, repoRoot, "status", "--porcelain=v1", "-z")
	if err != nil {
		return nil, fmt.Errorf("git status porcelain: %w", err)
	}
	return ParsePorcelainZ(output)
}

func (g *Git) TrackedFiles(ctx context.Context, repoRoot string) ([]domain.GitFile, error) {
	output, err := g.run(ctx, repoRoot, "ls-tree", "-r", "-z", "--full-tree", "-l", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("git ls-tree: %w", err)
	}
	var files []domain.GitFile
	for _, record := range bytes.Split(output, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		meta, pathBytes, ok := bytes.Cut(record, []byte{'\t'})
		if !ok {
			return nil, fmt.Errorf("invalid ls-tree record")
		}
		fields := strings.Fields(string(meta))
		if len(fields) < 4 {
			return nil, fmt.Errorf("invalid ls-tree metadata")
		}
		relative := normalizeRelative(string(pathBytes))
		if _, err := safeRepoPath(repoRoot, relative); err != nil {
			continue
		}
		mode, isSubmodule := gitMode(fields[0], fields[1])
		size := int64(0)
		if fields[3] != "-" {
			parsed, err := strconv.ParseInt(fields[3], 10, 64)
			if err == nil {
				size = parsed
			}
		}
		files = append(files, domain.GitFile{RelativePath: relative, Mode: mode, Size: size, IsSubmodule: isSubmodule})
	}
	return files, nil
}

func (g *Git) UntrackedFiles(ctx context.Context, repoRoot string) ([]domain.WorktreeFile, error) {
	output, err := g.run(ctx, repoRoot, "ls-files", "-o", "--exclude-standard", "-z")
	if err != nil {
		return nil, fmt.Errorf("git untracked files: %w", err)
	}
	return worktreeFilesFromZ(repoRoot, output)
}

func (g *Git) DirtyFiles(ctx context.Context, repoRoot string) ([]domain.WorktreeFile, error) {
	entries, err := g.StatusPorcelain(ctx, repoRoot)
	if err != nil {
		return nil, err
	}
	var paths []byte
	for _, entry := range entries {
		if IsDirty(entry) {
			paths = append(paths, []byte(entry.Path)...)
			paths = append(paths, 0)
		}
	}
	return worktreeFilesFromZ(repoRoot, paths)
}

func (g *Git) ReadBlob(ctx context.Context, repoRoot string, relativePath string) ([]byte, os.FileMode, error) {
	if _, err := safeRepoPath(repoRoot, relativePath); err != nil {
		return nil, 0, err
	}
	output, err := g.run(ctx, repoRoot, "show", "HEAD:"+relativePath)
	if err != nil {
		return nil, 0, fmt.Errorf("git show blob: %w", err)
	}
	mode := os.FileMode(0o644)
	files, err := g.TrackedFiles(ctx, repoRoot)
	if err == nil {
		for _, file := range files {
			if file.RelativePath == normalizeRelative(relativePath) {
				mode = file.Mode
				break
			}
		}
	}
	return output, mode, nil
}

func (g *Git) ApplyPatch(ctx context.Context, req domain.ApplyPatchRequest) error {
	if req.CreateBranch && req.BranchName != "" {
		if _, err := g.run(ctx, req.RepoPath, "checkout", "-b", req.BranchName); err != nil {
			return fmt.Errorf("create apply branch: %w", err)
		}
	}
	args := []string{"apply", "--3way", req.PatchPath}
	if _, err := g.run(ctx, req.RepoPath, args...); err != nil {
		if req.Reject {
			rejectArgs := []string{"apply", "--reject", req.PatchPath}
			if _, rejectErr := g.run(ctx, req.RepoPath, rejectArgs...); rejectErr == nil {
				return domain.ErrApplyConflict
			}
		}
		if _, reverseErr := g.run(ctx, req.RepoPath, "apply", "--reverse", "--check", req.PatchPath); reverseErr == nil {
			return nil
		}
		return domain.ErrApplyConflict
	}
	return nil
}

func (g *Git) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, g.executable, args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, domain.ErrDependency
		}
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return nil, fmt.Errorf("git %s failed: %w: %s", args[0], err, detail)
		}
		return nil, fmt.Errorf("git %s failed: %w", args[0], err)
	}
	return output, nil
}

func gitMode(mode string, objectType string) (os.FileMode, bool) {
	if objectType == "commit" || mode == "160000" {
		return 0, true
	}
	if mode == "120000" {
		return 0, true
	}
	if mode == "100755" {
		return 0o755, false
	}
	return 0o644, false
}

func worktreeFilesFromZ(repoRoot string, data []byte) ([]domain.WorktreeFile, error) {
	var files []domain.WorktreeFile
	for _, pathBytes := range bytes.Split(data, []byte{0}) {
		if len(pathBytes) == 0 {
			continue
		}
		relative := normalizeRelative(string(pathBytes))
		fullPath, err := safeRepoPath(repoRoot, relative)
		if err != nil {
			continue
		}
		info, err := os.Lstat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("stat worktree file: %w", err)
		}
		if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		files = append(files, domain.WorktreeFile{RelativePath: relative, Mode: info.Mode().Perm(), Size: info.Size()})
	}
	return files, nil
}
