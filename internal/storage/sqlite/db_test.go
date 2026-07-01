package sqlite

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenChmodsDB(t *testing.T) {
	t.Parallel()
	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "agentfence.db"), slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()
	info, err := os.Stat(filepath.Join(filepath.Dir(storePath(t, store)), "agentfence.db"))
	if err != nil {
		t.Fatalf("stat db: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("db mode=%v", info.Mode().Perm())
	}
}

func storePath(t *testing.T, store *Store) string {
	t.Helper()
	var value string
	if err := store.db.QueryRow("PRAGMA database_list").Scan(new(int), new(string), &value); err != nil {
		t.Fatalf("database_list: %v", err)
	}
	return value
}
