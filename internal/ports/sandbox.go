package ports

import (
	"context"

	"github.com/agentfence/agentfence/internal/domain"
)

type Sandbox interface {
	Run(ctx context.Context, req domain.SandboxRunRequest) (domain.SandboxRunResult, error)
}
