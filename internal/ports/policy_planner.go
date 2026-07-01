package ports

import (
	"context"

	"github.com/agentfence/agentfence/internal/domain"
)

type PolicyPlanner interface {
	BuildExposurePlan(ctx context.Context, req domain.ExposurePlanRequest) (domain.ExposurePlan, error)
}
