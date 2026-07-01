package sqlite

import (
	"context"
	"fmt"

	"github.com/agentfence/agentfence/internal/domain"
)

func (s *Store) CreateApplyAttempt(ctx context.Context, attempt domain.ApplyAttempt) (int64, error) {
	if attempt.CreatedAt.IsZero() {
		attempt.CreatedAt = s.clock.Now()
	}
	result, err := s.db.ExecContext(ctx, `
INSERT INTO apply_attempts(run_id, repo_path, patch_path, strategy, branch_name, reject_dir, status, error_code, error_message, created_at, finished_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		attempt.RunID, attempt.RepoPath, attempt.PatchPath, attempt.Strategy, attempt.BranchName, attempt.RejectDir,
		attempt.Status, attempt.ErrorCode, attempt.ErrorMessage, formatTime(attempt.CreatedAt), nullableTime(attempt.FinishedAt))
	if err != nil {
		return 0, fmt.Errorf("create apply attempt: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read apply attempt id: %w", err)
	}
	return id, nil
}

func (s *Store) FinishApplyAttempt(ctx context.Context, attemptID int64, status string, errorCode string, errorMessage string) error {
	now := s.clock.Now()
	result, err := s.db.ExecContext(ctx, `
UPDATE apply_attempts SET status = ?, error_code = ?, error_message = ?, finished_at = ? WHERE id = ?`,
		status, errorCode, errorMessage, formatTime(now), attemptID)
	if err != nil {
		return fmt.Errorf("finish apply attempt: %w", err)
	}
	return checkRowsAffected(result, domain.ErrRunNotFound)
}
