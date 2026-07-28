package policy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/ports"
)

type Planner struct {
	git ports.GitRunner
}

var _ ports.PolicyPlanner = (*Planner)(nil)

func NewPlanner(git ports.GitRunner) *Planner {
	return &Planner{git: git}
}

func (p *Planner) BuildExposurePlan(ctx context.Context, req domain.ExposurePlanRequest) (domain.ExposurePlan, error) {
	excludes := append([]string{}, req.Config.Workspace.Exclude...)
	excludes = append(excludes, MandatoryDenyPatterns()...)
	matcher, err := NewMatcher(req.Config.Workspace.Include, excludes)
	if err != nil {
		return domain.ExposurePlan{}, err
	}
	files := map[string]domain.ExposureFile{}
	var skipped []domain.SkippedFile
	tracked, err := p.git.TrackedFiles(ctx, req.RepoPath)
	if err != nil {
		return domain.ExposurePlan{}, fmt.Errorf("list tracked files: %w", err)
	}
	for _, file := range tracked {
		if file.IsSubmodule {
			skipped = append(skipped, domain.SkippedFile{RelativePath: file.RelativePath, Reason: "submodule_skipped", Size: file.Size})
			continue
		}
		exposure, ok, reason, err := p.evaluate(req, matcher, file.RelativePath, "head", file.Mode, file.Size, false)
		if err != nil {
			return domain.ExposurePlan{}, err
		}
		if !ok {
			skipped = append(skipped, domain.SkippedFile{RelativePath: file.RelativePath, Reason: reason, Size: file.Size})
			continue
		}
		files[exposure.RelativePath] = exposure
	}
	if req.IncludeUntracked || req.Config.Workspace.IncludeUntracked {
		untracked, err := p.git.UntrackedFiles(ctx, req.RepoPath)
		if err != nil {
			return domain.ExposurePlan{}, fmt.Errorf("list untracked files: %w", err)
		}
		p.addWorktreeFiles(req, matcher, files, &skipped, untracked, "untracked")
	}
	if req.IncludeDirty || req.Config.Workspace.IncludeDirty {
		dirty, err := p.git.DirtyFiles(ctx, req.RepoPath)
		if err != nil {
			return domain.ExposurePlan{}, fmt.Errorf("list dirty files: %w", err)
		}
		p.addWorktreeFiles(req, matcher, files, &skipped, dirty, "dirty")
	}
	result := domain.ExposurePlan{Files: make([]domain.ExposureFile, 0, len(files)), SkippedFiles: skipped}
	for _, file := range files {
		result.Files = append(result.Files, file)
		result.TotalSize += file.Size
	}
	sort.Slice(result.Files, func(i, j int) bool {
		return result.Files[i].RelativePath < result.Files[j].RelativePath
	})
	sort.Slice(result.SkippedFiles, func(i, j int) bool {
		return result.SkippedFiles[i].RelativePath < result.SkippedFiles[j].RelativePath
	})
	return result, nil
}

func (p *Planner) addWorktreeFiles(req domain.ExposurePlanRequest, matcher *Matcher, files map[string]domain.ExposureFile, skipped *[]domain.SkippedFile, worktree []domain.WorktreeFile, source string) {
	for _, file := range worktree {
		if file.Deleted {
			delete(files, filepath.ToSlash(file.RelativePath))
			continue
		}
		exposure, ok, reason, err := p.evaluate(req, matcher, file.RelativePath, source, file.Mode, file.Size, true)
		if err != nil {
			*skipped = append(*skipped, domain.SkippedFile{RelativePath: file.RelativePath, Reason: "unsafe_path", Size: file.Size})
			continue
		}
		if !ok {
			*skipped = append(*skipped, domain.SkippedFile{RelativePath: file.RelativePath, Reason: reason, Size: file.Size})
			continue
		}
		files[exposure.RelativePath] = exposure
	}
}

func (p *Planner) evaluate(req domain.ExposurePlanRequest, matcher *Matcher, relativePath string, source string, mode os.FileMode, size int64, checkSymlink bool) (domain.ExposureFile, bool, string, error) {
	fullPath, err := SafeJoin(req.RepoPath, relativePath)
	if err != nil {
		return domain.ExposureFile{}, false, "unsafe_path", nil
	}
	if checkSymlink {
		if mode&os.ModeSymlink != 0 {
			return domain.ExposureFile{}, false, "unsafe_symlink_blocked", nil
		}
		if err := ValidateSymlinkInside(req.RepoPath, fullPath); err != nil {
			return domain.ExposureFile{}, false, "unsafe_symlink_blocked", nil
		}
	}
	allowed, err := matcher.Allowed(relativePath)
	if err != nil {
		return domain.ExposureFile{}, false, "", err
	}
	if !allowed {
		return domain.ExposureFile{}, false, "policy_excluded", nil
	}
	if size > req.Config.Workspace.MaxFileSizeBytes {
		return domain.ExposureFile{}, false, "file_size_blocked", nil
	}
	return domain.ExposureFile{RelativePath: filepath.ToSlash(relativePath), Source: source, Mode: mode, Size: size}, true, "", nil
}
