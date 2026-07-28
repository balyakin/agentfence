package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/agentfence/agentfence/internal/config"
	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/errorsx"
	"github.com/agentfence/agentfence/internal/ports"
)

type ApplyService struct {
	store ports.Store
	git   ports.GitRunner
	clock ports.Clock
	locks ports.LockManager
}

func NewApplyService(store ports.Store, git ports.GitRunner, clock ports.Clock, locks ports.LockManager) (*ApplyService, error) {
	if store == nil || git == nil || clock == nil || locks == nil {
		return nil, errorsx.Wrap(errorsx.CodeInternal, "apply service dependency missing", errorsx.ExitInternal, nil)
	}
	return &ApplyService{store: store, git: git, clock: clock, locks: locks}, nil
}

func (s *ApplyService) Apply(ctx context.Context, req domain.ApplyRunRequest, repoPath string, configPath string) (domain.Run, error) {
	if req.Reject {
		return domain.Run{}, errorsx.Wrap(
			errorsx.CodeValidation,
			"reject mode is unsupported because it can partially modify the checkout",
			errorsx.ExitUsage,
			domain.ErrValidation,
		)
	}
	if req.RunID != "latest" {
		run, err := s.store.GetRun(ctx, req.RunID)
		if err != nil {
			return domain.Run{}, mapRunLoadError(err)
		}
		if repoPath == "" {
			repoPath = run.RepoPath
		}
	}
	repoRoot, err := s.git.FindRepoRoot(ctx, repoPath)
	if err != nil {
		return domain.Run{}, errorsx.Wrap(errorsx.CodeNotGitRepo, "not a git repository", errorsx.ExitUsage, err)
	}
	release, err := s.locks.Acquire(ctx, repoRoot)
	if err != nil {
		return domain.Run{}, errorsx.Wrap(errorsx.CodeActiveRunExists, "active run exists for repository", errorsx.ExitUsage, err)
	}
	defer release()
	var run domain.Run
	if req.RunID == "latest" {
		run, err = s.store.ResolveLatestRun(ctx, repoRoot)
	} else {
		run, err = s.store.GetRun(ctx, req.RunID)
	}
	if err != nil {
		return domain.Run{}, mapRunLoadError(err)
	}
	if run.RepoPath != repoRoot {
		return domain.Run{}, errorsx.Wrap(errorsx.CodeValidation, "run belongs to another repository", errorsx.ExitUsage, nil)
	}
	cfg, err := config.LoadForRepo(ctx, repoRoot, configPath)
	if err != nil {
		return domain.Run{}, err
	}
	if run.Status == domain.RunStatusBlockedPost {
		return domain.Run{}, errorsx.Wrap(
			errorsx.CodePostScanBlocked,
			"postflight scan did not produce an allowed result",
			errorsx.ExitSecurityBlocked,
			domain.ErrPostScanBlocked,
		)
	}
	if run.Status != domain.RunStatusSucceeded && run.Status != domain.RunStatusFailed {
		return domain.Run{}, errorsx.Wrap(errorsx.CodeValidation, "run status cannot be applied", errorsx.ExitUsage, domain.ErrValidation)
	}
	if run.PostScanStatus != "clean" && run.PostScanStatus != "findings" {
		return domain.Run{}, errorsx.Wrap(
			errorsx.CodePostScanBlocked,
			"postflight scan did not produce an allowed result",
			errorsx.ExitSecurityBlocked,
			domain.ErrPostScanBlocked,
		)
	}
	patchInfo, err := os.Lstat(run.PatchPath)
	if err != nil {
		return domain.Run{}, errorsx.Wrap(errorsx.CodeNotFound, "patch file not found", errorsx.ExitNotFound, err)
	}
	if !patchInfo.Mode().IsRegular() {
		return domain.Run{}, errorsx.Wrap(errorsx.CodeValidation, "patch path is not a regular file", errorsx.ExitUsage, domain.ErrValidation)
	}
	if patchInfo.Size() == 0 {
		return domain.Run{}, errorsx.Wrap(errorsx.CodeValidation, "patch file is empty", errorsx.ExitUsage, domain.ErrValidation)
	}
	currentBranch, err := s.git.BaseRef(ctx, repoRoot)
	if err != nil {
		return domain.Run{}, fmt.Errorf("check current branch: %w", err)
	}
	if run.BaseRef != "" && currentBranch != run.BaseRef {
		data, marshalErr := json.Marshal(map[string]string{
			"expected_branch": run.BaseRef,
			"actual_branch":   currentBranch,
			"base_ref":        run.BaseRef,
		})
		if marshalErr == nil {
			eventErr := s.store.InsertEvent(ctx, domain.RunEvent{RunID: run.ID, Ts: s.clock.Now(), Level: "warn", EventType: "branch_mismatch", Message: "current branch differs from run base", DataJSON: string(data)})
			if eventErr != nil {
				suppressBestEffortError(eventErr)
			}
		}
	}
	if cfg.Apply.RequireCleanTree && !req.AllowDirtyTree {
		status, err := s.git.StatusPorcelain(ctx, repoRoot)
		if err != nil {
			return domain.Run{}, err
		}
		if len(status) > 0 {
			return domain.Run{}, errorsx.Wrap(errorsx.CodeRepoDirty, "repository is dirty", errorsx.ExitUsage, domain.ErrRepoDirty)
		}
	}
	createBranch := req.CreateBranch || cfg.Apply.CreateBranch
	branchName := req.BranchName
	if createBranch && branchName == "" {
		short := run.ID
		if len(short) > 10 {
			short = short[:10]
		}
		branchName = strings.TrimRight(cfg.Apply.BranchPrefix, "/") + "/" + short
	}
	transitioned, err := s.store.TransitionRunStatus(
		ctx,
		run.ID,
		[]domain.RunStatus{domain.RunStatusSucceeded, domain.RunStatusFailed},
		domain.RunStatusApplying,
	)
	if err != nil {
		return domain.Run{}, err
	}
	if !transitioned {
		return domain.Run{}, errorsx.Wrap(
			errorsx.CodeValidation,
			"run status changed before apply",
			errorsx.ExitUsage,
			domain.ErrValidation,
		)
	}
	applyFinished := false
	defer func() {
		if !applyFinished {
			if err := s.store.SetRunStatus(context.WithoutCancel(ctx), run.ID, domain.RunStatusApplyFailed); err != nil {
				suppressBestEffortError(err)
			}
		}
	}()
	attemptID, err := s.store.CreateApplyAttempt(ctx, domain.ApplyAttempt{
		RunID: run.ID, RepoPath: repoRoot, PatchPath: run.PatchPath, Strategy: "git_apply",
		BranchName: branchName, Status: "started", CreatedAt: s.clock.Now(),
	})
	if err != nil {
		return domain.Run{}, err
	}
	applyReq := domain.ApplyPatchRequest{
		RepoPath: repoRoot, PatchPath: run.PatchPath, CreateBranch: createBranch,
		BranchName: branchName, BaseRef: currentBranch,
	}
	if err := s.git.ApplyPatch(ctx, applyReq); err != nil {
		code := errorsx.CodeApplyConflict
		if !errors.Is(err, domain.ErrApplyConflict) {
			code = errorsx.CodeInternal
		}
		finishErr := s.store.FinishApplyAttempt(ctx, attemptID, "failed", code, "apply failed")
		if finishErr != nil {
			return domain.Run{}, finishErr
		}
		statusErr := s.store.SetRunStatus(ctx, run.ID, domain.RunStatusApplyFailed)
		if statusErr != nil {
			return domain.Run{}, statusErr
		}
		applyFinished = true
		return domain.Run{}, errorsx.Wrap(code, "apply failed", errorsx.ExitApplyConflict, err)
	}
	if err := s.store.FinishApplyAttempt(ctx, attemptID, "succeeded", "", ""); err != nil {
		return domain.Run{}, s.rollbackAfterPersistenceFailure(ctx, run.ID, attemptID, applyReq, err)
	}
	if err := s.store.SetRunStatus(ctx, run.ID, domain.RunStatusApplied); err != nil {
		return domain.Run{}, s.rollbackAfterPersistenceFailure(ctx, run.ID, attemptID, applyReq, err)
	}
	applyFinished = true
	run.Status = domain.RunStatusApplied
	return run, nil
}

func (s *ApplyService) rollbackAfterPersistenceFailure(
	ctx context.Context,
	runID string,
	attemptID int64,
	req domain.ApplyPatchRequest,
	persistenceErr error,
) error {
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if err := s.git.RollbackPatch(rollbackCtx, req); err != nil {
		return fmt.Errorf("persist apply result: %w; rollback checkout: %v", persistenceErr, err)
	}
	if err := s.store.FinishApplyAttempt(
		rollbackCtx,
		attemptID,
		"rolled_back",
		errorsx.CodeInternal,
		"checkout rolled back after persistence failure",
	); err != nil {
		return fmt.Errorf("persist apply result: %w; persist rollback attempt: %v", persistenceErr, err)
	}
	if err := s.store.SetRunStatus(rollbackCtx, runID, domain.RunStatusApplyFailed); err != nil {
		return fmt.Errorf("persist apply result: %w; persist rollback status: %v", persistenceErr, err)
	}
	return fmt.Errorf("persist apply result after checkout rollback: %w", persistenceErr)
}

func suppressBestEffortError(error) {}
