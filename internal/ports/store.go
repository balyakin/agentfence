package ports

import (
	"context"

	"github.com/agentfence/agentfence/internal/domain"
)

type Store interface {
	CreateRun(ctx context.Context, run domain.Run) error
	GetRun(ctx context.Context, runID string) (domain.Run, error)
	ResolveLatestRun(ctx context.Context, repoPath string) (domain.Run, error)
	ListRuns(ctx context.Context, filter domain.RunFilter) ([]domain.RunSummary, error)
	SetRunStatus(ctx context.Context, runID string, status domain.RunStatus) error
	TransitionRunStatus(
		ctx context.Context,
		runID string,
		from []domain.RunStatus,
		to domain.RunStatus,
	) (bool, error)
	SetRunStarted(ctx context.Context, runID string) error
	SetRunFinished(ctx context.Context, result domain.RunFinishResult) error
	SetScanStatus(ctx context.Context, runID string, preStatus string, postStatus string) error
	InsertEvent(ctx context.Context, event domain.RunEvent) error
	InsertFindings(ctx context.Context, findings []domain.Finding) error
	SaveConfigSnapshot(ctx context.Context, runID string, configJSON string, effectivePolicyJSON string) error
	CreateApplyAttempt(ctx context.Context, attempt domain.ApplyAttempt) (int64, error)
	FinishApplyAttempt(ctx context.Context, attemptID int64, status string, errorCode string, errorMessage string) error
}
