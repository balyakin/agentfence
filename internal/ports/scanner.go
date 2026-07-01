package ports

import (
	"context"

	"github.com/agentfence/agentfence/internal/domain"
)

type Scanner interface {
	Name() string
	Scan(ctx context.Context, req domain.ScanRequest) (domain.ScanResult, error)
}
