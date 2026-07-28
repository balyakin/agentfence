package state

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentfence/agentfence/internal/domain"
)

func TestResolveConfiguredPaths(t *testing.T) {
	// ARRANGE
	rootDir := t.TempDir()
	stateDir := filepath.Join(rootDir, "state")
	cacheDir := filepath.Join(rootDir, "cache")

	// ACT
	paths, err := Resolve(context.Background(), stateDir, cacheDir)

	// ASSERT
	if err != nil {
		t.Fatalf("resolve paths: %v", err)
	}
	if paths.StateDir != stateDir {
		t.Fatalf("state dir = %q, want %q", paths.StateDir, stateDir)
	}
	if paths.CacheDir != cacheDir {
		t.Fatalf("cache dir = %q, want %q", paths.CacheDir, cacheDir)
	}
	if paths.RunDir("run-1") != filepath.Join(cacheDir, "runs", "run-1") {
		t.Fatalf("unexpected run dir: %q", paths.RunDir("run-1"))
	}
	if paths.LockPath("/repo") == paths.LockPath("/other") {
		t.Fatal("different repositories share a lock path")
	}
	if RepoHash("/repo") == RepoHash("/other") {
		t.Fatal("different repositories share a hash")
	}
	for _, dir := range []string{paths.StateDir, paths.CacheDir, paths.RunsDir, paths.LocksDir} {
		info, statErr := os.Stat(dir)
		if statErr != nil {
			t.Fatalf("stat %q: %v", dir, statErr)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("mode for %q = %o, want 700", dir, info.Mode().Perm())
		}
	}
}

func TestResolveUsesXDGPaths(t *testing.T) {
	// ARRANGE
	rootDir := t.TempDir()
	stateHome := filepath.Join(rootDir, "state-home")
	cacheHome := filepath.Join(rootDir, "cache-home")
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	// ACT
	paths, err := Resolve(context.Background(), "", "")

	// ASSERT
	if err != nil {
		t.Fatalf("resolve XDG paths: %v", err)
	}
	if paths.StateDir != filepath.Join(stateHome, "agentfence") {
		t.Fatalf("unexpected state dir: %q", paths.StateDir)
	}
	if paths.CacheDir != filepath.Join(cacheHome, "agentfence") {
		t.Fatalf("unexpected cache dir: %q", paths.CacheDir)
	}
}

func TestResolveCanceled(t *testing.T) {
	// ARRANGE
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// ACT
	_, err := Resolve(ctx, t.TempDir(), t.TempDir())

	// ASSERT
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}

func TestCleanupRunDirs(t *testing.T) {
	// ARRANGE
	cacheDir := t.TempDir()
	runDir := filepath.Join(cacheDir, "runs", "run-1")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatalf("create run dir: %v", err)
	}
	paths := Paths{CacheDir: cacheDir}
	candidates := []CleanupCandidate{
		{RunID: "run-1", RunDir: runDir},
		{RunID: "missing", RunDir: filepath.Join(cacheDir, "runs", "missing")},
		{RunID: "outside", RunDir: filepath.Join(filepath.Dir(cacheDir), "outside")},
	}

	// ACT
	dryResult, err := CleanupRunDirs(context.Background(), paths, candidates, true)
	if err != nil {
		t.Fatalf("dry-run cleanup: %v", err)
	}
	result, err := CleanupRunDirs(context.Background(), paths, candidates, false)

	// ASSERT
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if len(dryResult.Deleted) != 1 || dryResult.Deleted[0] != runDir {
		t.Fatalf("unexpected dry-run result: %#v", dryResult)
	}
	if len(result.Deleted) != 1 || result.Deleted[0] != runDir {
		t.Fatalf("unexpected cleanup result: %#v", result)
	}
	if len(result.Skipped) != 2 {
		t.Fatalf("unexpected skipped paths: %#v", result.Skipped)
	}
	if _, statErr := os.Stat(runDir); !os.IsNotExist(statErr) {
		t.Fatalf("run dir was not removed: %v", statErr)
	}
}

func TestCleanupRunDirsCanceled(t *testing.T) {
	// ARRANGE
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// ACT
	_, err := CleanupRunDirs(ctx, Paths{CacheDir: t.TempDir()}, []CleanupCandidate{{RunID: "run-1"}}, false)

	// ASSERT
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}

func TestFileLockManagerRejectsConcurrentLock(t *testing.T) {
	// ARRANGE
	paths := Paths{LocksDir: t.TempDir()}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	manager := NewFileLockManager(paths, logger)
	release, err := manager.Acquire(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("acquire first lock: %v", err)
	}
	defer release()

	// ACT
	_, err = manager.Acquire(context.Background(), "/repo")

	// ASSERT
	if !errors.Is(err, domain.ErrActiveRunExists) {
		t.Fatalf("error = %v, want active run exists", err)
	}
}

func TestFileLockManagerHonorsCancellation(t *testing.T) {
	// ARRANGE
	paths := Paths{LocksDir: t.TempDir()}
	manager := NewFileLockManager(paths, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// ACT
	_, err := manager.Acquire(ctx, "/repo")

	// ASSERT
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}
