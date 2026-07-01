package sqlite

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentfence/agentfence/internal/domain"
)

func TestResolveLatestIgnoresActive(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	defer store.Close()
	now := time.Now().UTC()
	active := testRun("active", domain.RunStatusRunning, now)
	done := testRun("done", domain.RunStatusSucceeded, now.Add(time.Second))
	if err := store.CreateRun(context.Background(), active); err != nil {
		t.Fatalf("create active: %v", err)
	}
	if err := store.CreateRun(context.Background(), done); err != nil {
		t.Fatalf("create done: %v", err)
	}
	run, err := store.ResolveLatestRun(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if run.ID != "done" {
		t.Fatalf("latest=%s", run.ID)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "agentfence.db"), slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return store
}

func testRun(id string, status domain.RunStatus, now time.Time) domain.Run {
	return domain.Run{ID: id, RepoPath: "/repo", RunDir: "/run/" + id, ShadowPath: "/run/" + id + "/shadow", MetadataPath: "/run/" + id + "/shadow_metadata.json", PatchPath: "/run/" + id + "/changes.patch", AgentName: "agent", TaskRedacted: "task", Status: status, PreScanStatus: "clean", PostScanStatus: "clean", NetworkMode: "deny", TimeoutSeconds: 1, CreatedAt: now, UpdatedAt: now}
}
