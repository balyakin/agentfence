package sqlite

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
)

func TestMigrationsIdempotent(t *testing.T) {
	t.Parallel()
	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "agentfence.db"), slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate second time: %v", err)
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(1) FROM schema_migrations WHERE version = 1`).Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != 1 {
		t.Fatalf("migration count=%d", count)
	}
}
