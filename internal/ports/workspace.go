package ports

import (
	"context"
	"os"

	"github.com/agentfence/agentfence/internal/domain"
)

type WorkspaceManager interface {
	Create(ctx context.Context, req domain.CreateWorkspaceRequest) (domain.WorkspaceResult, error)
	GeneratePatch(ctx context.Context, req domain.GeneratePatchRequest) error
	ValidatePostRunWorkspace(ctx context.Context, shadowPath string) error
}

type BlobReader interface {
	Read(ctx context.Context, repoRoot string, file domain.ExposureFile) ([]byte, os.FileMode, error)
}
