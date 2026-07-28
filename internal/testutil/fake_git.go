package testutil

import (
	"context"

	"github.com/agentfence/agentfence/internal/domain"
)

type FakeGit struct {
	RepoRoot       string
	Dirty          []domain.StatusEntry
	Tracked        []domain.GitFile
	Untracked      []domain.WorktreeFile
	DirtyFilesList []domain.WorktreeFile
	Applied        bool
	RolledBack     bool
}

func (g *FakeGit) FindRepoRoot(ctx context.Context, startDir string) (string, error) {
	if g.RepoRoot == "" {
		return startDir, nil
	}
	return g.RepoRoot, nil
}
func (g *FakeGit) HeadSHA(ctx context.Context, repoRoot string) (string, error) {
	return "HEADSHA", nil
}
func (g *FakeGit) BaseRef(ctx context.Context, repoRoot string) (string, error) { return "main", nil }
func (g *FakeGit) StatusPorcelain(ctx context.Context, repoRoot string) ([]domain.StatusEntry, error) {
	return g.Dirty, nil
}
func (g *FakeGit) TrackedFiles(ctx context.Context, repoRoot string) ([]domain.GitFile, error) {
	if g.Tracked != nil {
		return g.Tracked, nil
	}
	return []domain.GitFile{{RelativePath: "main.go", Mode: 0o644, Size: 12}}, nil
}
func (g *FakeGit) UntrackedFiles(ctx context.Context, repoRoot string) ([]domain.WorktreeFile, error) {
	return g.Untracked, nil
}
func (g *FakeGit) DirtyFiles(ctx context.Context, repoRoot string) ([]domain.WorktreeFile, error) {
	return g.DirtyFilesList, nil
}
func (g *FakeGit) ReadBlob(ctx context.Context, repoRoot string, relativePath string) ([]byte, error) {
	return []byte("package main\n"), nil
}
func (g *FakeGit) ApplyPatch(ctx context.Context, req domain.ApplyPatchRequest) error {
	g.Applied = true
	return nil
}

func (g *FakeGit) RollbackPatch(ctx context.Context, req domain.ApplyPatchRequest) error {
	g.RolledBack = true
	return nil
}
