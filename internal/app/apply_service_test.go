package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/ports"
	"github.com/agentfence/agentfence/internal/testutil"
)

func TestApplyServiceAppliesSucceededRun(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeStore()
	git := &testutil.FakeGit{RepoRoot: "/repo"}
	patch := filepath.Join(t.TempDir(), "changes.patch")
	if err := os.WriteFile(patch, []byte("diff"), 0o600); err != nil {
		t.Fatalf("write patch: %v", err)
	}
	now := time.Now().UTC()
	run := domain.Run{ID: "01ARZ3NDEKTSV4RRFFQ69G5FAV", RepoPath: "/repo", PatchPath: patch, RunDir: filepath.Dir(patch), Status: domain.RunStatusSucceeded, PostScanStatus: "clean", CreatedAt: now, UpdatedAt: now}
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	locks := &applyLockManager{}
	service, err := NewApplyService(store, git, ports.SystemClock{}, locks)
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	_, err = service.Apply(context.Background(), domain.ApplyRunRequest{RunID: run.ID}, "/repo", "")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !git.Applied {
		t.Fatalf("git apply not called")
	}
	if !locks.acquired || !locks.released {
		t.Fatalf("repo lock was not held during apply")
	}
}

func TestApplyServiceRejectsEmptyPatch(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeStore()
	git := &testutil.FakeGit{RepoRoot: "/repo"}
	patch := filepath.Join(t.TempDir(), "changes.patch")
	if err := os.WriteFile(patch, nil, 0o600); err != nil {
		t.Fatalf("write patch: %v", err)
	}
	now := time.Now().UTC()
	run := domain.Run{ID: "01ARZ3NDEKTSV4RRFFQ69G5FAW", RepoPath: "/repo", PatchPath: patch, RunDir: filepath.Dir(patch), Status: domain.RunStatusFailed, PostScanStatus: "clean", CreatedAt: now, UpdatedAt: now}
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	service, err := NewApplyService(store, git, ports.SystemClock{}, &applyLockManager{})
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	_, err = service.Apply(context.Background(), domain.ApplyRunRequest{RunID: run.ID}, "/repo", "")
	if err == nil {
		t.Fatalf("expected empty patch rejection")
	}
	if git.Applied {
		t.Fatalf("git apply should not be called")
	}
}

func TestApplyServiceRollsBackWhenAppliedStatusCannotPersist(t *testing.T) {
	t.Parallel()
	baseStore := testutil.NewFakeStore()
	store := &failingAppliedStore{FakeStore: baseStore}
	git := &testutil.FakeGit{RepoRoot: "/repo"}
	patch := filepath.Join(t.TempDir(), "changes.patch")
	if err := os.WriteFile(patch, []byte("diff"), 0o600); err != nil {
		t.Fatalf("write patch: %v", err)
	}
	now := time.Now().UTC()
	run := domain.Run{
		ID: "01ARZ3NDEKTSV4RRFFQ69G5FAY", RepoPath: "/repo", PatchPath: patch,
		Status: domain.RunStatusSucceeded, PostScanStatus: "clean", CreatedAt: now, UpdatedAt: now,
	}
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	service, err := NewApplyService(store, git, ports.SystemClock{}, &applyLockManager{})
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	_, err = service.Apply(context.Background(), domain.ApplyRunRequest{RunID: run.ID}, "/repo", "")
	if err == nil {
		t.Fatalf("expected persistence failure")
	}
	if !git.RolledBack {
		t.Fatalf("checkout was not rolled back")
	}
}

type failingAppliedStore struct {
	*testutil.FakeStore
}

func (s *failingAppliedStore) SetRunStatus(
	ctx context.Context,
	runID string,
	status domain.RunStatus,
) error {
	if status == domain.RunStatusApplied {
		return errors.New("persist applied failed")
	}
	return s.FakeStore.SetRunStatus(ctx, runID, status)
}

type applyLockManager struct {
	acquired bool
	released bool
}

func (m *applyLockManager) Acquire(ctx context.Context, repoPath string) (func(), error) {
	m.acquired = true
	return func() {
		m.released = true
	}, nil
}
