package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type migration struct {
	version int
	name    string
	sql     string
}

func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("ensure schema migrations table: %w", err)
	}
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}
	for _, item := range migrations {
		item := item
		if err := s.withTx(ctx, func(txCtx context.Context, tx *sql.Tx) error {
			var count int
			if err := tx.QueryRowContext(txCtx, `SELECT COUNT(1) FROM schema_migrations WHERE version = ?`, item.version).Scan(&count); err != nil {
				return fmt.Errorf("check migration version: %w", err)
			}
			if count > 0 {
				return nil
			}
			if _, err := tx.ExecContext(txCtx, item.sql); err != nil {
				return fmt.Errorf("execute migration %s: %w", item.name, err)
			}
			if _, err := tx.ExecContext(txCtx, `INSERT INTO schema_migrations(version, name, applied_at) VALUES (?, ?, ?)`, item.version, item.name, formatTime(s.clock.Now())); err != nil {
				return fmt.Errorf("insert migration row: %w", err)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func loadMigrations() ([]migration, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	var migrations []migration
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		versionPart, _, ok := strings.Cut(entry.Name(), "_")
		if !ok {
			return nil, fmt.Errorf("invalid migration name %s", entry.Name())
		}
		version, err := strconv.Atoi(versionPart)
		if err != nil {
			return nil, fmt.Errorf("parse migration version: %w", err)
		}
		data, err := migrationsFS.ReadFile(path.Join("migrations", entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read migration: %w", err)
		}
		migrations = append(migrations, migration{version: version, name: entry.Name(), sql: string(data)})
	}
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})
	return migrations, nil
}
