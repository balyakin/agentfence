package api

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/agentfence/agentfence/internal/app"
	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/errorsx"
	"github.com/agentfence/agentfence/internal/ports"
	"golang.org/x/sync/errgroup"
)

const reconciliationPageSize = 1000

type RunJob struct {
	RunID string
	Req   domain.RunRequest
}

type RunQueue struct {
	jobs      chan RunJob
	done      chan struct{}
	cancel    map[string]context.CancelFunc
	cancelled map[string]struct{}
	pending   map[string]struct{}
	mu        sync.RWMutex
	group     *errgroup.Group
	service   *app.RunService
	store     ports.Store
	logger    *slog.Logger
	ctx       context.Context
	stopped   bool
}

func NewRunQueue(
	ctx context.Context,
	maxWorkers int,
	service *app.RunService,
	store ports.Store,
	logger *slog.Logger,
) (*RunQueue, error) {
	if maxWorkers <= 0 {
		maxWorkers = 1
	}
	group, groupCtx := errgroup.WithContext(context.WithoutCancel(ctx))
	queue := &RunQueue{
		jobs: make(chan RunJob, maxWorkers*2), done: make(chan struct{}), cancel: map[string]context.CancelFunc{},
		cancelled: map[string]struct{}{}, pending: map[string]struct{}{}, group: group,
		service: service, store: store, logger: logger, ctx: groupCtx,
	}
	if err := queue.reconcileActiveRuns(ctx); err != nil {
		return nil, err
	}
	for i := 0; i < maxWorkers; i++ {
		group.Go(func() error {
			return queue.worker(groupCtx)
		})
	}
	return queue, nil
}

func (q *RunQueue) Enqueue(ctx context.Context, job RunJob) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.stopped {
		return errorsx.Wrap(errorsx.CodeInternal, "run queue stopped", errorsx.ExitInternal, nil)
	}
	select {
	case q.jobs <- job:
		q.pending[job.RunID] = struct{}{}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return errorsx.Wrap(errorsx.CodeBusy, "run queue is full", errorsx.ExitInternal, nil)
	}
}

func (q *RunQueue) Cancel(runID string) bool {
	q.mu.Lock()
	_, queued := q.pending[runID]
	cancel, running := q.cancel[runID]
	if !queued && !running {
		q.mu.Unlock()
		return false
	}
	q.cancelled[runID] = struct{}{}
	q.mu.Unlock()
	if running {
		cancel()
	}
	return true
}

func (q *RunQueue) Stop() error {
	q.mu.Lock()
	if !q.stopped {
		q.stopped = true
		close(q.done)
		for _, cancel := range q.cancel {
			cancel()
		}
	}
	q.mu.Unlock()
	return q.group.Wait()
}

func (q *RunQueue) worker(ctx context.Context) error {
	for {
		select {
		case <-q.done:
			q.drain(ctx)
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case job := <-q.jobs:
			q.runJob(ctx, job)
		}
	}
}

func (q *RunQueue) drain(ctx context.Context) {
	for {
		select {
		case job := <-q.jobs:
			q.deletePending(job.RunID)
			q.finishCancelledBeforeStart(ctx, job.RunID)
		default:
			return
		}
	}
}

func (q *RunQueue) runJob(ctx context.Context, job RunJob) {
	q.mu.Lock()
	delete(q.pending, job.RunID)
	if q.stopped {
		delete(q.cancelled, job.RunID)
		q.mu.Unlock()
		q.finishCancelledBeforeStart(ctx, job.RunID)
		return
	}
	if _, ok := q.cancelled[job.RunID]; ok {
		delete(q.cancelled, job.RunID)
		q.mu.Unlock()
		q.finishCancelledBeforeStart(ctx, job.RunID)
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	q.cancel[job.RunID] = cancel
	q.mu.Unlock()
	defer func() {
		q.mu.Lock()
		delete(q.cancel, job.RunID)
		delete(q.cancelled, job.RunID)
		q.mu.Unlock()
		cancel()
		if recovered := recover(); recovered != nil {
			if q.logger != nil {
				q.logger.ErrorContext(ctx, "run queue worker recovered", slog.String("run_id", job.RunID), slog.Any("error", fmt.Sprint(recovered)))
			}
			if q.store != nil {
				err := q.store.SetRunFinished(context.WithoutCancel(ctx), domain.RunFinishResult{
					RunID: job.RunID, Status: domain.RunStatusFailed, ErrorCode: errorsx.CodeInternal, ErrorMessage: "queued run failed", FinishedAt: time.Now().UTC(),
				})
				if err != nil && q.logger != nil {
					q.logger.ErrorContext(ctx, "mark recovered queued run failed", slog.String("run_id", job.RunID), slog.Any("error", err))
				}
			}
		}
	}()
	job.Req.RunID = job.RunID
	if _, err := q.service.Run(runCtx, job.Req); err != nil && q.logger != nil {
		q.logger.ErrorContext(ctx, "queued run failed", slog.String("run_id", job.RunID), slog.Any("error", err))
	}
}

func (q *RunQueue) deletePending(runID string) {
	q.mu.Lock()
	delete(q.pending, runID)
	q.mu.Unlock()
}

func (q *RunQueue) finishCancelledBeforeStart(ctx context.Context, runID string) {
	if q.store == nil {
		return
	}
	err := q.store.SetRunFinished(context.WithoutCancel(ctx), domain.RunFinishResult{
		RunID: runID, Status: domain.RunStatusCancelled, ErrorCode: errorsx.CodeCancelled,
		ErrorMessage: "run cancelled before start", FinishedAt: time.Now().UTC(),
	})
	if err != nil && q.logger != nil {
		q.logger.ErrorContext(ctx, "mark queued run cancelled failed", slog.String("run_id", runID), slog.Any("error", err))
	}
}

func (q *RunQueue) reconcileActiveRuns(ctx context.Context) error {
	if q.store == nil {
		return nil
	}
	statuses := []domain.RunStatus{
		domain.RunStatusCreated,
		domain.RunStatusPreparing,
		domain.RunStatusScanning,
		domain.RunStatusRunning,
		domain.RunStatusPostScan,
		domain.RunStatusApplying,
	}
	for _, status := range statuses {
		runs, err := q.listRunsByStatus(ctx, status)
		if err != nil {
			return err
		}
		for _, run := range runs {
			terminalStatus := domain.RunStatusFailed
			if status == domain.RunStatusApplying {
				terminalStatus = domain.RunStatusApplyFailed
			}
			finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			err := q.store.SetRunFinished(finishCtx, domain.RunFinishResult{
				RunID: run.ID, Status: terminalStatus, ErrorCode: errorsx.CodeInternal,
				ErrorMessage: "daemon restarted before run completed", FinishedAt: time.Now().UTC(),
			})
			cancel()
			if err != nil && q.logger != nil {
				q.logger.ErrorContext(ctx, "reconcile queued run failed", slog.String("run_id", run.ID), slog.Any("error", err))
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (q *RunQueue) listRunsByStatus(ctx context.Context, status domain.RunStatus) ([]domain.RunSummary, error) {
	var runs []domain.RunSummary
	for {
		page, err := q.store.ListRuns(ctx, domain.RunFilter{
			Status: status,
			Limit:  reconciliationPageSize,
			Offset: len(runs),
		})
		if err != nil {
			if q.logger != nil {
				q.logger.ErrorContext(ctx, "reconcile queued runs failed", slog.String("status", string(status)), slog.Any("error", err))
			}
			return nil, err
		}
		runs = append(runs, page...)
		if len(page) < reconciliationPageSize {
			return runs, nil
		}
	}
}
