package app

import (
	"context"
	"time"

	"github.com/agentfence/agentfence/internal/config"
	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/errorsx"
	"github.com/agentfence/agentfence/internal/ports"
	"github.com/agentfence/agentfence/internal/state"
)

type CleanService struct {
	store     ports.Store
	paths     state.Paths
	retention config.RetentionConfig
	clock     ports.Clock
}

func NewCleanService(store ports.Store, paths state.Paths, retention config.RetentionConfig, clock ports.Clock) (*CleanService, error) {
	if store == nil || paths.CacheDir == "" || paths.RunsDir == "" || clock == nil {
		return nil, errorsx.Wrap(errorsx.CodeInternal, "clean service dependency missing", errorsx.ExitInternal, nil)
	}
	return &CleanService{store: store, paths: paths, retention: retention, clock: clock}, nil
}

func (s *CleanService) Clean(ctx context.Context, repoPath string, dryRun bool) (state.CleanupResult, error) {
	runs, err := s.store.ListRuns(ctx, domain.RunFilter{RepoPath: repoPath, Limit: 1000})
	if err != nil {
		return state.CleanupResult{}, err
	}
	cutoff := s.clock.Now().Add(-time.Duration(s.retention.MaxAgeDays) * 24 * time.Hour)
	terminalSeen := 0
	var candidates []state.CleanupCandidate
	for _, run := range runs {
		if !run.Status.IsTerminal() {
			continue
		}
		terminalSeen++
		if s.retention.KeepFailed && isFailedForRetention(run.Status) {
			continue
		}
		tooMany := s.retention.MaxRunsPerRepo > 0 && terminalSeen > s.retention.MaxRunsPerRepo
		tooOld := run.FinishedAt != nil && run.FinishedAt.Before(cutoff)
		if tooMany || tooOld {
			candidates = append(candidates, state.CleanupCandidate{RunID: run.ID, RunDir: s.paths.RunDir(run.ID)})
		}
	}
	return state.CleanupRunDirs(ctx, s.paths, candidates, dryRun)
}

func isFailedForRetention(status domain.RunStatus) bool {
	switch status {
	case domain.RunStatusFailed, domain.RunStatusApplyFailed, domain.RunStatusTimedOut, domain.RunStatusCancelled:
		return true
	default:
		return false
	}
}
