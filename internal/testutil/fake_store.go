package testutil

import (
	"context"
	"sync"

	"github.com/agentfence/agentfence/internal/domain"
)

type FakeStore struct {
	mu        sync.Mutex
	Runs      map[string]domain.Run
	Findings  []domain.Finding
	Events    []domain.RunEvent
	Snapshots map[string]string
}

func NewFakeStore() *FakeStore {
	return &FakeStore{Runs: map[string]domain.Run{}, Snapshots: map[string]string{}}
}

func (s *FakeStore) CreateRun(ctx context.Context, run domain.Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Runs[run.ID] = run
	return nil
}

func (s *FakeStore) GetRun(ctx context.Context, runID string) (domain.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.Runs[runID]
	if !ok {
		return domain.Run{}, domain.ErrRunNotFound
	}
	return run, nil
}

func (s *FakeStore) ResolveLatestRun(ctx context.Context, repoPath string) (domain.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, run := range s.Runs {
		if run.RepoPath == repoPath && !run.Status.IsActive() {
			return run, nil
		}
	}
	return domain.Run{}, domain.ErrRunNotFound
}

func (s *FakeStore) ListRuns(ctx context.Context, filter domain.RunFilter) ([]domain.RunSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []domain.RunSummary
	for _, run := range s.Runs {
		if filter.RepoPath != "" && run.RepoPath != filter.RepoPath {
			continue
		}
		result = append(result, domain.RunSummary{ID: run.ID, RepoPath: run.RepoPath, AgentName: run.AgentName, Status: run.Status, TaskRedacted: run.TaskRedacted, CreatedAt: run.CreatedAt, FinishedAt: run.FinishedAt})
	}
	return result, nil
}

func (s *FakeStore) SetRunStatus(ctx context.Context, runID string, status domain.RunStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.Runs[runID]
	if !ok {
		return domain.ErrRunNotFound
	}
	run.Status = status
	s.Runs[runID] = run
	return nil
}

func (s *FakeStore) SetRunStarted(ctx context.Context, runID string) error { return nil }

func (s *FakeStore) SetRunFinished(ctx context.Context, result domain.RunFinishResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.Runs[result.RunID]
	if !ok {
		return domain.ErrRunNotFound
	}
	run.Status = result.Status
	run.ExitCode = result.ExitCode
	if result.IsolationLevel != "" {
		run.IsolationLevel = result.IsolationLevel
	}
	run.ErrorCode = result.ErrorCode
	run.ErrorMessage = result.ErrorMessage
	run.FinishedAt = &result.FinishedAt
	s.Runs[result.RunID] = run
	return nil
}

func (s *FakeStore) SetRunContext(ctx context.Context, runID string, repoHead string, baseRef string, networkMode string, timeoutSeconds int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.Runs[runID]
	if !ok {
		return domain.ErrRunNotFound
	}
	run.RepoHead = repoHead
	run.BaseRef = baseRef
	run.NetworkMode = networkMode
	run.TimeoutSeconds = timeoutSeconds
	s.Runs[runID] = run
	return nil
}

func (s *FakeStore) SetScanStatus(ctx context.Context, runID string, preStatus string, postStatus string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run := s.Runs[runID]
	run.PreScanStatus = preStatus
	run.PostScanStatus = postStatus
	s.Runs[runID] = run
	return nil
}

func (s *FakeStore) InsertEvent(ctx context.Context, event domain.RunEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Events = append(s.Events, event)
	return nil
}

func (s *FakeStore) InsertFindings(ctx context.Context, findings []domain.Finding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Findings = append(s.Findings, findings...)
	return nil
}

func (s *FakeStore) SaveConfigSnapshot(ctx context.Context, runID string, configJSON string, effectivePolicyJSON string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Snapshots[runID] = configJSON + effectivePolicyJSON
	return nil
}

func (s *FakeStore) CreateApplyAttempt(ctx context.Context, attempt domain.ApplyAttempt) (int64, error) {
	return 1, nil
}

func (s *FakeStore) FinishApplyAttempt(ctx context.Context, attemptID int64, status string, errorCode string, errorMessage string) error {
	return nil
}
