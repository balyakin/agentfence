package policy

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentfence/agentfence/internal/config"
	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/testutil"
)

func TestExposurePlanExcludesEnv(t *testing.T) {
	t.Parallel()
	git := &testutil.FakeGit{RepoRoot: "/repo"}
	planner := NewPlanner(git)
	cfg := config.DefaultConfig()
	plan, err := planner.BuildExposurePlan(context.Background(), domain.ExposurePlanRequest{RepoPath: "/repo", Config: cfg})
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if len(plan.Files) != 1 || plan.Files[0].RelativePath != "main.go" {
		t.Fatalf("unexpected plan: %#v", plan.Files)
	}
}

func TestExposurePlanKeepsMandatoryExcludesAfterReplace(t *testing.T) {
	t.Parallel()
	git := &testutil.FakeGit{
		RepoRoot: "/repo",
		Tracked: []domain.GitFile{
			{RelativePath: "main.go", Mode: 0o644, Size: 12},
			{RelativePath: "nested/.ENV", Mode: 0o600, Size: 12},
		},
	}
	cfg := config.DefaultConfig()
	cfg.Workspace.Exclude = []string{"build/**"}
	cfg.Workspace.ExcludeReplace = true
	plan, err := NewPlanner(git).BuildExposurePlan(
		context.Background(),
		domain.ExposurePlanRequest{RepoPath: "/repo", Config: cfg},
	)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if len(plan.Files) != 1 || plan.Files[0].RelativePath != "main.go" {
		t.Fatalf("mandatory excludes were replaced: %#v", plan.Files)
	}
}

func TestExposurePlanRejectsWorktreeSymlink(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	target := filepath.Join(repoRoot, ".env")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(repoRoot, "notes.txt")
	if err := os.Symlink(".env", link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	git := &testutil.FakeGit{
		RepoRoot: repoRoot,
		Tracked:  []domain.GitFile{},
		Untracked: []domain.WorktreeFile{
			{RelativePath: "notes.txt", Mode: os.ModeSymlink | 0o777, Size: 4},
		},
	}
	cfg := config.DefaultConfig()
	cfg.Workspace.IncludeUntracked = true
	plan, err := NewPlanner(git).BuildExposurePlan(
		context.Background(),
		domain.ExposurePlanRequest{RepoPath: repoRoot, Config: cfg},
	)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if len(plan.Files) != 0 {
		t.Fatalf("worktree symlink was exposed: %#v", plan.Files)
	}
}

func TestExposurePlanRespectsDirtyDeletion(t *testing.T) {
	t.Parallel()
	git := &testutil.FakeGit{
		RepoRoot: "/repo",
		Tracked: []domain.GitFile{
			{RelativePath: "deleted.txt", Mode: 0o644, Size: 12},
		},
		DirtyFilesList: []domain.WorktreeFile{
			{RelativePath: "deleted.txt", Deleted: true},
		},
	}
	cfg := config.DefaultConfig()
	cfg.Workspace.IncludeDirty = true
	plan, err := NewPlanner(git).BuildExposurePlan(
		context.Background(),
		domain.ExposurePlanRequest{RepoPath: "/repo", Config: cfg},
	)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if len(plan.Files) != 0 {
		t.Fatalf("deleted file was restored into shadow: %#v", plan.Files)
	}
}

func TestExposurePlanClassifiesSkippedFiles(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "large.txt"), []byte("large"), 0o600); err != nil {
		t.Fatalf("write large file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "allowed.txt"), []byte("ok"), 0o600); err != nil {
		t.Fatalf("write allowed file: %v", err)
	}
	git := &testutil.FakeGit{
		Tracked: []domain.GitFile{
			{RelativePath: "module", IsSubmodule: true},
			{RelativePath: ".env", Mode: 0o600, Size: 10},
		},
		Untracked: []domain.WorktreeFile{
			{RelativePath: "large.txt", Mode: 0o600, Size: 5},
			{RelativePath: "allowed.txt", Mode: 0o600, Size: 2},
		},
	}
	cfg := config.DefaultConfig()
	cfg.Workspace.IncludeUntracked = true
	cfg.Workspace.MaxFileSizeBytes = 4

	plan, err := NewPlanner(git).BuildExposurePlan(
		context.Background(),
		domain.ExposurePlanRequest{RepoPath: repoRoot, Config: cfg},
	)

	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if len(plan.Files) != 1 || plan.Files[0].RelativePath != "allowed.txt" {
		t.Fatalf("unexpected files: %#v", plan.Files)
	}
	if len(plan.SkippedFiles) != 3 {
		t.Fatalf("unexpected skipped files: %#v", plan.SkippedFiles)
	}
}
