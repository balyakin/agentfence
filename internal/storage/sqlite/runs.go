package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/agentfence/agentfence/internal/domain"
)

func (s *Store) CreateRun(ctx context.Context, run domain.Run) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO runs (
	id, repo_path, repo_head, base_ref, run_dir, shadow_path, metadata_path, patch_path,
	agent_name, task_redacted, status, pre_scan_status, post_scan_status, network_mode,
	isolation_level, timeout_seconds, exit_code, error_code, error_message, created_at,
	started_at, finished_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.RepoPath, run.RepoHead, run.BaseRef, run.RunDir, run.ShadowPath, run.MetadataPath, run.PatchPath,
		run.AgentName, run.TaskRedacted, string(run.Status), defaultString(run.PreScanStatus, "pending"),
		defaultString(run.PostScanStatus, "pending"), run.NetworkMode, run.IsolationLevel, run.TimeoutSeconds,
		run.ExitCode, run.ErrorCode, run.ErrorMessage, formatTime(run.CreatedAt), nullableTime(run.StartedAt),
		nullableTime(run.FinishedAt), formatTime(run.UpdatedAt))
	if err != nil {
		return fmt.Errorf("create run: %w", err)
	}
	return nil
}

func (s *Store) GetRun(ctx context.Context, runID string) (domain.Run, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, repo_path, repo_head, base_ref, run_dir, shadow_path, metadata_path, patch_path,
	agent_name, task_redacted, status, pre_scan_status, post_scan_status, network_mode,
	isolation_level, timeout_seconds, exit_code, error_code, error_message, created_at,
	started_at, finished_at, updated_at
FROM runs WHERE id = ?`, runID)
	run, err := scanRun(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Run{}, domain.ErrRunNotFound
		}
		return domain.Run{}, fmt.Errorf("get run: %w", err)
	}
	return run, nil
}

func (s *Store) ResolveLatestRun(ctx context.Context, repoPath string) (domain.Run, error) {
	active := []string{
		string(domain.RunStatusCreated),
		string(domain.RunStatusPreparing),
		string(domain.RunStatusScanning),
		string(domain.RunStatusRunning),
		string(domain.RunStatusPostScan),
		string(domain.RunStatusApplying),
		string(domain.RunStatusCleaned),
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(active)), ",")
	args := []any{repoPath}
	for _, status := range active {
		args = append(args, status)
	}
	query := `
SELECT id, repo_path, repo_head, base_ref, run_dir, shadow_path, metadata_path, patch_path,
	agent_name, task_redacted, status, pre_scan_status, post_scan_status, network_mode,
	isolation_level, timeout_seconds, exit_code, error_code, error_message, created_at,
	started_at, finished_at, updated_at
FROM runs WHERE repo_path = ? AND status NOT IN (` + placeholders + `)
ORDER BY created_at DESC, id DESC LIMIT 1`
	run, err := scanRun(s.db.QueryRowContext(ctx, query, args...))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Run{}, domain.ErrRunNotFound
		}
		return domain.Run{}, fmt.Errorf("resolve latest run: %w", err)
	}
	return run, nil
}

func (s *Store) ListRuns(ctx context.Context, filter domain.RunFilter) ([]domain.RunSummary, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	query := `SELECT id, repo_path, agent_name, status, task_redacted, created_at, finished_at FROM runs`
	var args []any
	var where []string
	if filter.RepoPath != "" {
		where = append(where, "repo_path = ?")
		args = append(args, filter.RepoPath)
	}
	if filter.Status != "" {
		where = append(where, "status = ?")
		args = append(args, string(filter.Status))
	}
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?"
	args = append(args, limit)
	args = append(args, filter.Offset)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && s.logger != nil {
			s.logger.ErrorContext(ctx, "close run rows failed", slog.Any("error", err))
		}
	}()
	var result []domain.RunSummary
	for rows.Next() {
		var summary domain.RunSummary
		var status string
		var created string
		var finished sql.NullString
		if err := rows.Scan(&summary.ID, &summary.RepoPath, &summary.AgentName, &status, &summary.TaskRedacted, &created, &finished); err != nil {
			return nil, fmt.Errorf("scan run summary: %w", err)
		}
		createdAt, err := parseTime(created)
		if err != nil {
			return nil, fmt.Errorf("parse run created_at: %w", err)
		}
		finishedAt, err := parseNullTime(finished)
		if err != nil {
			return nil, fmt.Errorf("parse run finished_at: %w", err)
		}
		summary.Status = domain.RunStatus(status)
		summary.CreatedAt = createdAt
		summary.FinishedAt = finishedAt
		result = append(result, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runs: %w", err)
	}
	return result, nil
}

func (s *Store) SetRunStatus(ctx context.Context, runID string, status domain.RunStatus) error {
	result, err := s.db.ExecContext(ctx, `UPDATE runs SET status = ?, updated_at = ? WHERE id = ?`, string(status), formatTime(s.clock.Now()), runID)
	if err != nil {
		return fmt.Errorf("set run status: %w", err)
	}
	return checkRowsAffected(result, domain.ErrRunNotFound)
}

func (s *Store) TransitionRunStatus(
	ctx context.Context,
	runID string,
	from []domain.RunStatus,
	to domain.RunStatus,
) (bool, error) {
	if len(from) == 0 {
		return false, fmt.Errorf("transition run status requires source statuses")
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(from)), ",")
	args := []any{string(to), formatTime(s.clock.Now()), runID}
	for _, status := range from {
		args = append(args, string(status))
	}
	query := `UPDATE runs SET status = ?, updated_at = ? WHERE id = ? AND status IN (` + placeholders + `)`
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("transition run status: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check transition rows: %w", err)
	}
	return rows == 1, nil
}

func (s *Store) SetRunStarted(ctx context.Context, runID string) error {
	now := formatTime(s.clock.Now())
	result, err := s.db.ExecContext(ctx, `UPDATE runs SET started_at = ?, updated_at = ? WHERE id = ?`, now, now, runID)
	if err != nil {
		return fmt.Errorf("set run started: %w", err)
	}
	return checkRowsAffected(result, domain.ErrRunNotFound)
}

func (s *Store) SetRunFinished(ctx context.Context, result domain.RunFinishResult) error {
	exitCode := result.ExitCode
	dbResult, err := s.db.ExecContext(ctx, `
UPDATE runs SET status = ?, exit_code = ?, isolation_level = CASE WHEN ? = '' THEN isolation_level ELSE ? END,
	error_code = ?, error_message = ?, finished_at = ?, updated_at = ?
WHERE id = ?`,
		string(result.Status), exitCode, result.IsolationLevel, result.IsolationLevel, result.ErrorCode, result.ErrorMessage,
		formatTime(result.FinishedAt), formatTime(result.FinishedAt), result.RunID)
	if err != nil {
		return fmt.Errorf("set run finished: %w", err)
	}
	return checkRowsAffected(dbResult, domain.ErrRunNotFound)
}

func (s *Store) SetRunContext(ctx context.Context, runID string, repoHead string, baseRef string, networkMode string, timeoutSeconds int) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE runs SET repo_head = ?, base_ref = ?, network_mode = ?, timeout_seconds = ?, updated_at = ?
WHERE id = ?`,
		repoHead, baseRef, networkMode, timeoutSeconds, formatTime(s.clock.Now()), runID)
	if err != nil {
		return fmt.Errorf("set run context: %w", err)
	}
	return checkRowsAffected(result, domain.ErrRunNotFound)
}

func (s *Store) SetScanStatus(ctx context.Context, runID string, preStatus string, postStatus string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE runs SET pre_scan_status = ?, post_scan_status = ?, updated_at = ? WHERE id = ?`, preStatus, postStatus, formatTime(s.clock.Now()), runID)
	if err != nil {
		return fmt.Errorf("set scan status: %w", err)
	}
	return checkRowsAffected(result, domain.ErrRunNotFound)
}

type runScanner interface {
	Scan(dest ...any) error
}

func scanRun(row runScanner) (domain.Run, error) {
	var run domain.Run
	var status string
	var exitCode sql.NullInt64
	var created string
	var updated string
	var started sql.NullString
	var finished sql.NullString
	if err := row.Scan(
		&run.ID, &run.RepoPath, &run.RepoHead, &run.BaseRef, &run.RunDir, &run.ShadowPath, &run.MetadataPath, &run.PatchPath,
		&run.AgentName, &run.TaskRedacted, &status, &run.PreScanStatus, &run.PostScanStatus, &run.NetworkMode,
		&run.IsolationLevel, &run.TimeoutSeconds, &exitCode, &run.ErrorCode, &run.ErrorMessage, &created,
		&started, &finished, &updated,
	); err != nil {
		return domain.Run{}, err
	}
	run.Status = domain.RunStatus(status)
	if exitCode.Valid {
		value := int(exitCode.Int64)
		run.ExitCode = &value
	}
	createdAt, err := parseTime(created)
	if err != nil {
		return domain.Run{}, fmt.Errorf("parse created_at: %w", err)
	}
	updatedAt, err := parseTime(updated)
	if err != nil {
		return domain.Run{}, fmt.Errorf("parse updated_at: %w", err)
	}
	startedAt, err := parseNullTime(started)
	if err != nil {
		return domain.Run{}, fmt.Errorf("parse started_at: %w", err)
	}
	finishedAt, err := parseNullTime(finished)
	if err != nil {
		return domain.Run{}, fmt.Errorf("parse finished_at: %w", err)
	}
	run.CreatedAt = createdAt
	run.UpdatedAt = updatedAt
	run.StartedAt = startedAt
	run.FinishedAt = finishedAt
	return run, nil
}

func nullableTime(ts *time.Time) any {
	if ts == nil {
		return nil
	}
	return formatTime(*ts)
}

func defaultString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func checkRowsAffected(result sql.Result, notFound error) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rows == 0 {
		return notFound
	}
	return nil
}
