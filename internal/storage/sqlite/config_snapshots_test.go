package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/agentfence/agentfence/internal/domain"
)

func TestSaveConfigSnapshot(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	defer store.Close()
	run := testRun("snapshot", domain.RunStatusSucceeded, time.Now().UTC())
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := store.SaveConfigSnapshot(context.Background(), run.ID, `{"version":1}`, `{"files":[]}`); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(1) FROM config_snapshots WHERE run_id = ?`, run.ID).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("snapshot count=%d", count)
	}
}
