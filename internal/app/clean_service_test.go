package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentfence/agentfence/internal/config"
	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/errorsx"
	"github.com/agentfence/agentfence/internal/ports"
	"github.com/agentfence/agentfence/internal/state"
	"github.com/agentfence/agentfence/internal/testutil"
)

func TestCleanServiceConstructor(t *testing.T) {
	t.Parallel()
	if _, err := NewCleanService(nil, state.Paths{}, config.DefaultConfig().Retention, ports.SystemClock{}); err == nil {
		t.Fatalf("expected missing dependency error")
	} else if public, ok := errorsx.IsPublic(err); !ok || public.Code != errorsx.CodeInternal {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCleanServiceMarksRemovedRunCleaned(t *testing.T) {
	t.Parallel()
	cacheDir := t.TempDir()
	paths := state.Paths{CacheDir: cacheDir, RunsDir: filepath.Join(cacheDir, "runs")}
	runID := "run"
	runDir := paths.RunDir(runID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatalf("create run dir: %v", err)
	}
	now := time.Now().UTC()
	finished := now.Add(-48 * time.Hour)
	store := testutil.NewFakeStore()
	if err := store.CreateRun(context.Background(), domain.Run{
		ID: runID, RepoPath: "/repo", Status: domain.RunStatusSucceeded,
		CreatedAt: finished, FinishedAt: &finished, UpdatedAt: finished,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	retention := config.DefaultConfig().Retention
	retention.MaxAgeDays = 1
	service, err := NewCleanService(store, paths, retention, fixedClock{now: now})
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	if _, err := service.Clean(context.Background(), "/repo", false); err != nil {
		t.Fatalf("clean: %v", err)
	}
	run, err := store.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != domain.RunStatusCleaned {
		t.Fatalf("status=%s", run.Status)
	}
}

func TestCleanServiceDoesNotMarkSkippedRunCleaned(t *testing.T) {
	t.Parallel()
	cacheDir := t.TempDir()
	paths := state.Paths{CacheDir: cacheDir, RunsDir: filepath.Join(cacheDir, "runs")}
	now := time.Now().UTC()
	finished := now.Add(-48 * time.Hour)
	store := testutil.NewFakeStore()
	if err := store.CreateRun(context.Background(), domain.Run{
		ID: "missing", RepoPath: "/repo", Status: domain.RunStatusSucceeded,
		CreatedAt: finished, FinishedAt: &finished, UpdatedAt: finished,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	retention := config.DefaultConfig().Retention
	retention.MaxAgeDays = 1
	service, err := NewCleanService(store, paths, retention, fixedClock{now: now})
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	result, err := service.Clean(context.Background(), "/repo", false)
	if err != nil {
		t.Fatalf("clean: %v", err)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("unexpected cleanup result: %#v", result)
	}
	run, err := store.GetRun(context.Background(), "missing")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != domain.RunStatusSucceeded {
		t.Fatalf("skipped run status = %s", run.Status)
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}
