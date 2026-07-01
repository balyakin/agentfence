package app

import (
	"context"
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
