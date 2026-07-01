package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/agentfence/agentfence/internal/ports"
	_ "modernc.org/sqlite"
)

type Store struct {
	db     *sql.DB
	logger *slog.Logger
	clock  ports.Clock
}

var _ ports.Store = (*Store)(nil)

func sqliteDSN(path string) string {
	return "file:" + path +
		"?_pragma=foreign_keys(1)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_txlock=immediate"
}

func Open(ctx context.Context, path string, logger *slog.Logger, clock ports.Clock) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create sqlite directory: %w", err)
	}
	database, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	database.SetMaxOpenConns(8)
	database.SetMaxIdleConns(3)
	database.SetConnMaxLifetime(time.Hour)

	if err := database.PingContext(ctx); err != nil {
		closeErr := database.Close()
		if closeErr != nil && logger != nil {
			logger.ErrorContext(ctx, "close sqlite after ping failure failed", slog.Any("error", closeErr))
		}
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		closeErr := database.Close()
		if closeErr != nil && logger != nil {
			logger.ErrorContext(ctx, "close sqlite after chmod failure failed", slog.Any("error", closeErr))
		}
		return nil, fmt.Errorf("chmod sqlite db: %w", err)
	}
	if clock == nil {
		clock = ports.SystemClock{}
	}
	store := &Store{db: database, logger: logger, clock: clock}
	if err := store.Migrate(ctx); err != nil {
		closeErr := database.Close()
		if closeErr != nil && logger != nil {
			logger.ErrorContext(ctx, "close sqlite after migration failure failed", slog.Any("error", closeErr))
		}
		return nil, fmt.Errorf("migrate sqlite: %w", err)
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) withTx(ctx context.Context, fn func(context.Context, *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	if err := fn(ctx, tx); err != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil && s.logger != nil {
			s.logger.ErrorContext(ctx, "rollback failed", slog.Any("error", rollbackErr))
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func formatTime(ts time.Time) string {
	return ts.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}

func parseNullTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid || value.String == "" {
		return nil, nil
	}
	ts, err := parseTime(value.String)
	if err != nil {
		return nil, err
	}
	return &ts, nil
}
