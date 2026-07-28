package ports

import (
	"context"

	"github.com/agentfence/agentfence/internal/domain"
)

type GitRunner interface {
	FindRepoRoot(ctx context.Context, startDir string) (string, error)
	HeadSHA(ctx context.Context, repoRoot string) (string, error)
	BaseRef(ctx context.Context, repoRoot string) (string, error)
	StatusPorcelain(ctx context.Context, repoRoot string) ([]domain.StatusEntry, error)
	TrackedFiles(ctx context.Context, repoRoot string) ([]domain.GitFile, error)
	UntrackedFiles(ctx context.Context, repoRoot string) ([]domain.WorktreeFile, error)
	DirtyFiles(ctx context.Context, repoRoot string) ([]domain.WorktreeFile, error)
	ReadBlob(ctx context.Context, repoRoot string, relativePath string) ([]byte, error)
	ApplyPatch(ctx context.Context, req domain.ApplyPatchRequest) error
	RollbackPatch(ctx context.Context, req domain.ApplyPatchRequest) error
}
