package sqlite

import (
	"context"
	"database/sql"
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
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})
	info, err := os.Stat(filepath.Join(filepath.Dir(storePath(t, store)), "agentfence.db"))
	if err != nil {
		t.Fatalf("stat db: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("db mode=%v", info.Mode().Perm())
	}
}

func TestOpenSupportsReservedPathCharacters(t *testing.T) {
	t.Parallel()
	directory := filepath.Join(t.TempDir(), "state?part#fragment")
	path := filepath.Join(directory, "agentfence?data#.db")
	store, err := Open(
		context.Background(),
		path,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		nil,
	)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("database created at wrong path: %v", err)
	}
}

func TestWithTxRollsBackPanic(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	func() {
		defer func() {
			_ = recover()
		}()
		_ = store.withTx(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "CREATE TABLE panic_test(value TEXT)"); err != nil {
				t.Fatalf("create table: %v", err)
			}
			panic("test panic")
		})
	}()
	if _, err := store.db.Exec("CREATE TABLE panic_test(value TEXT)"); err != nil {
		t.Fatalf("transaction remained open after panic: %v", err)
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
