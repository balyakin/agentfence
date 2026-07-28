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

func TestResolveLatestUsesChronologicalTimeAndIDTieBreaker(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	base := time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC)
	runs := []domain.Run{
		testRun("a", domain.RunStatusSucceeded, base.Add(9*time.Nanosecond)),
		testRun("b", domain.RunStatusSucceeded, base.Add(10*time.Nanosecond)),
		testRun("c", domain.RunStatusSucceeded, base.Add(10*time.Nanosecond)),
	}
	for _, run := range runs {
		if err := store.CreateRun(context.Background(), run); err != nil {
			t.Fatalf("create %s: %v", run.ID, err)
		}
	}
	latest, err := store.ResolveLatestRun(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if latest.ID != "c" {
		t.Fatalf("latest=%s", latest.ID)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "agentfence.db"), slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})
	return store
}

func testRun(id string, status domain.RunStatus, now time.Time) domain.Run {
	return domain.Run{ID: id, RepoPath: "/repo", RunDir: "/run/" + id, ShadowPath: "/run/" + id + "/shadow", MetadataPath: "/run/" + id + "/shadow_metadata.json", PatchPath: "/run/" + id + "/changes.patch", AgentName: "agent", TaskRedacted: "task", Status: status, PreScanStatus: "clean", PostScanStatus: "clean", NetworkMode: "deny", TimeoutSeconds: 1, CreatedAt: now, UpdatedAt: now}
}
