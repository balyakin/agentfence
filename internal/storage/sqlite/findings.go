package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/agentfence/agentfence/internal/domain"
)

func (s *Store) InsertFindings(ctx context.Context, findings []domain.Finding) error {
	if len(findings) == 0 {
		return nil
	}
	return s.withTx(ctx, func(txCtx context.Context, tx *sql.Tx) error {
		for _, finding := range findings {
			createdAt := finding.CreatedAt
			if createdAt.IsZero() {
				createdAt = s.clock.Now()
			}
			if _, err := tx.ExecContext(txCtx, `
INSERT INTO scan_findings(run_id, phase, engine, file_path, line, column_number, rule_id, severity, fingerprint, secret_sha256, redacted_secret, description, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				finding.RunID, string(finding.Phase), finding.Engine, finding.FilePath, finding.Line, finding.ColumnNumber,
				finding.RuleID, finding.Severity, finding.Fingerprint, finding.SecretSHA256, finding.RedactedSecret,
				finding.Description, formatTime(createdAt)); err != nil {
				return fmt.Errorf("insert scan finding: %w", err)
			}
		}
		return nil
	})
}
