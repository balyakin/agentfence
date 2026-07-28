package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agentfence/agentfence/internal/domain"
)

func TestStoreRunLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := openTestStore(t)
	now := time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC)
	run := testRun("run-lifecycle", domain.RunStatusCreated, now)
	run.PreScanStatus = ""
	run.PostScanStatus = ""

	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	created, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if created.PreScanStatus != "pending" || created.PostScanStatus != "pending" {
		t.Fatalf("default scan statuses were not stored: %#v", created)
	}
	if err := store.SetRunStarted(ctx, run.ID); err != nil {
		t.Fatalf("set started: %v", err)
	}
	if err := store.SetRunContext(ctx, run.ID, "head", "main", "deny", 30); err != nil {
		t.Fatalf("set context: %v", err)
	}
	if err := store.SetScanStatus(ctx, run.ID, "clean", "findings"); err != nil {
		t.Fatalf("set scan status: %v", err)
	}
	transitioned, err := store.TransitionRunStatus(
		ctx,
		run.ID,
		[]domain.RunStatus{domain.RunStatusCreated},
		domain.RunStatusRunning,
	)
	if err != nil {
		t.Fatalf("transition status: %v", err)
	}
	if !transitioned {
		t.Fatal("expected status transition")
	}
	transitioned, err = store.TransitionRunStatus(
		ctx,
		run.ID,
		[]domain.RunStatus{domain.RunStatusCreated},
		domain.RunStatusFailed,
	)
	if err != nil {
		t.Fatalf("check rejected transition: %v", err)
	}
	if transitioned {
		t.Fatal("transition from stale status succeeded")
	}
	exitCode := 7
	finishedAt := now.Add(time.Minute)
	if err := store.SetRunFinished(ctx, domain.RunFinishResult{
		RunID:          run.ID,
		Status:         domain.RunStatusFailed,
		ExitCode:       &exitCode,
		IsolationLevel: "hard",
		ErrorCode:      "AF_TEST",
		ErrorMessage:   "failed",
		FinishedAt:     finishedAt,
	}); err != nil {
		t.Fatalf("finish run: %v", err)
	}
	finished, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get finished run: %v", err)
	}
	if finished.Status != domain.RunStatusFailed || finished.ExitCode == nil || *finished.ExitCode != exitCode {
		t.Fatalf("unexpected finished run: %#v", finished)
	}
	if finished.IsolationLevel != "hard" || finished.FinishedAt == nil {
		t.Fatalf("missing finish details: %#v", finished)
	}
}

func TestStoreListsAndRejectsMissingRuns(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := openTestStore(t)
	now := time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC)
	for _, run := range []domain.Run{
		testRun("run-a", domain.RunStatusSucceeded, now),
		testRun("run-b", domain.RunStatusFailed, now.Add(time.Second)),
		testRun("run-c", domain.RunStatusFailed, now.Add(2*time.Second)),
	} {
		if err := store.CreateRun(ctx, run); err != nil {
			t.Fatalf("create run %s: %v", run.ID, err)
		}
	}
	runs, err := store.ListRuns(ctx, domain.RunFilter{
		RepoPath: "/repo",
		Status:   domain.RunStatusFailed,
		Limit:    1,
		Offset:   1,
	})
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != "run-b" {
		t.Fatalf("unexpected page: %#v", runs)
	}
	if _, err := store.GetRun(ctx, "missing"); !errors.Is(err, domain.ErrRunNotFound) {
		t.Fatalf("get missing error = %v", err)
	}
	if _, err := store.ResolveLatestRun(ctx, "/missing"); !errors.Is(err, domain.ErrRunNotFound) {
		t.Fatalf("latest missing error = %v", err)
	}
	for _, operation := range []func() error{
		func() error {
			return store.SetRunStatus(ctx, "missing", domain.RunStatusFailed)
		},
		func() error {
			return store.SetRunStarted(ctx, "missing")
		},
		func() error {
			return store.SetRunContext(ctx, "missing", "head", "main", "deny", 1)
		},
		func() error {
			return store.SetScanStatus(ctx, "missing", "clean", "clean")
		},
		func() error {
			return store.SetRunFinished(ctx, domain.RunFinishResult{
				RunID: "missing", Status: domain.RunStatusFailed, FinishedAt: now,
			})
		},
	} {
		if err := operation(); !errors.Is(err, domain.ErrRunNotFound) {
			t.Fatalf("missing run error = %v", err)
		}
	}
	if _, err := store.TransitionRunStatus(ctx, "run-a", nil, domain.RunStatusFailed); err == nil {
		t.Fatal("empty source statuses were accepted")
	}
}

func TestStorePersistsRelatedRecords(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := openTestStore(t)
	now := time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC)
	run := testRun("run-related", domain.RunStatusSucceeded, now)
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := store.InsertEvent(ctx, domain.RunEvent{
		RunID: run.ID, Level: "info", EventType: "test", Message: "stored",
	}); err != nil {
		t.Fatalf("insert event: %v", err)
	}
	if err := store.InsertFindings(ctx, nil); err != nil {
		t.Fatalf("insert empty findings: %v", err)
	}
	if err := store.InsertFindings(ctx, []domain.Finding{{
		RunID: run.ID, Phase: domain.FindingPhasePatch, Engine: "scanner", FilePath: "file.txt",
		Line: 1, ColumnNumber: 2, RuleID: "rule", Severity: domain.SeverityHigh,
		Fingerprint: "fingerprint", SecretSHA256: "hash", RedactedSecret: "****", Description: "finding",
	}}); err != nil {
		t.Fatalf("insert finding: %v", err)
	}
	attemptID, err := store.CreateApplyAttempt(ctx, domain.ApplyAttempt{
		RunID: run.ID, RepoPath: run.RepoPath, PatchPath: run.PatchPath, Strategy: "apply", Status: "running",
	})
	if err != nil {
		t.Fatalf("create apply attempt: %v", err)
	}
	if err := store.FinishApplyAttempt(ctx, attemptID, "succeeded", "", ""); err != nil {
		t.Fatalf("finish apply attempt: %v", err)
	}
	if err := store.FinishApplyAttempt(ctx, attemptID+1000, "failed", "AF_TEST", "missing"); !errors.Is(
		err,
		domain.ErrRunNotFound,
	) {
		t.Fatalf("finish missing attempt error = %v", err)
	}
	for table, expected := range map[string]int{
		"run_events":     1,
		"scan_findings":  1,
		"apply_attempts": 1,
	} {
		var count int
		if err := store.DB().QueryRow("SELECT COUNT(1) FROM " + table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != expected {
			t.Fatalf("%s count = %d, want %d", table, count, expected)
		}
	}
}
