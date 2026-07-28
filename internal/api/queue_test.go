package api

import (
	"context"
	"testing"
	"time"

	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/errorsx"
	"github.com/agentfence/agentfence/internal/testutil"
)

func TestRunQueueCancelMissing(t *testing.T) {
	t.Parallel()
	queue, err := NewRunQueue(context.Background(), 1, nil, nil, nil)
	if err != nil {
		t.Fatalf("new queue: %v", err)
	}
	defer func() {
		_ = queue.Stop()
	}()
	if queue.Cancel("missing") {
		t.Fatalf("missing run cancelled")
	}
}

func TestRunQueueEnqueueAfterStopReturnsError(t *testing.T) {
	t.Parallel()
	queue, err := NewRunQueue(context.Background(), 1, nil, nil, nil)
	if err != nil {
		t.Fatalf("new queue: %v", err)
	}
	if err := queue.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("enqueue panicked: %v", recovered)
		}
	}()
	err = queue.Enqueue(context.Background(), RunJob{RunID: "run"})
	if err == nil {
		t.Fatalf("expected enqueue error")
	}
}

func TestRunQueueCancelQueuedRun(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	runID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	store := testutil.NewFakeStore()
	if err := store.CreateRun(ctx, domain.Run{ID: runID, Status: domain.RunStatusCreated}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	queue := &RunQueue{
		jobs: make(chan RunJob, 1), done: make(chan struct{}), cancel: map[string]context.CancelFunc{},
		cancelled: map[string]struct{}{}, pending: map[string]struct{}{}, store: store,
	}
	if err := queue.Enqueue(ctx, RunJob{RunID: runID}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if !queue.Cancel(runID) {
		t.Fatalf("queued run was not cancelled")
	}
	queue.runJob(ctx, RunJob{RunID: runID})
	run, err := store.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != domain.RunStatusCancelled || run.ErrorCode != errorsx.CodeCancelled {
		t.Fatalf("run was not marked cancelled: %#v", run)
	}
}

func TestRunQueueDoesNotStartJobAfterStop(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	runID := "01ARZ3NDEKTSV4RRFFQ69G5FAY"
	store := testutil.NewFakeStore()
	if err := store.CreateRun(ctx, domain.Run{ID: runID, Status: domain.RunStatusCreated}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	queue := &RunQueue{
		jobs: make(chan RunJob, 1), done: make(chan struct{}), cancel: map[string]context.CancelFunc{},
		cancelled: map[string]struct{}{}, pending: map[string]struct{}{runID: {}}, store: store, stopped: true,
	}

	queue.runJob(ctx, RunJob{RunID: runID})

	run, err := store.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != domain.RunStatusCancelled {
		t.Fatalf("run status = %s, want cancelled", run.Status)
	}
}

func TestRunQueueReconcilesActiveRuns(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := testutil.NewFakeStore()
	runID := "01ARZ3NDEKTSV4RRFFQ69G5FAX"
	if err := store.CreateRun(ctx, domain.Run{
		ID: runID, Status: domain.RunStatusRunning, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	queue, err := NewRunQueue(ctx, 1, nil, store, nil)
	if err != nil {
		t.Fatalf("new queue: %v", err)
	}
	defer func() {
		if err := queue.Stop(); err != nil {
			t.Fatalf("stop: %v", err)
		}
	}()
	run, err := store.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != domain.RunStatusFailed || run.FinishedAt == nil {
		t.Fatalf("active run was not reconciled: %#v", run)
	}
}
