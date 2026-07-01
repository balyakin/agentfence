package sqlite

import (
	"context"
	"fmt"
)

func (s *Store) SaveConfigSnapshot(ctx context.Context, runID string, configJSON string, effectivePolicyJSON string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO config_snapshots(run_id, config_json, effective_policy_json, created_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(run_id) DO UPDATE SET config_json = excluded.config_json, effective_policy_json = excluded.effective_policy_json, created_at = excluded.created_at`,
		runID, configJSON, effectivePolicyJSON, formatTime(s.clock.Now()))
	if err != nil {
		return fmt.Errorf("save config snapshot: %w", err)
	}
	return nil
}
