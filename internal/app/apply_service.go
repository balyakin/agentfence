package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	var run domain.Run
	var err error
	if req.RunID != "latest" {
		run, err = s.store.GetRun(ctx, req.RunID)
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
	cfg, err := config.LoadForRepo(ctx, repoRoot, configPath)
	if err != nil {
		return domain.Run{}, err
	}
	if req.RunID == "latest" {
		run, err = s.store.ResolveLatestRun(ctx, repoRoot)
		if err != nil {
			return domain.Run{}, mapRunLoadError(err)
		}
	}
	if run.RepoPath != repoRoot {
		return domain.Run{}, errorsx.Wrap(errorsx.CodeValidation, "run belongs to another repository", errorsx.ExitUsage, nil)
	}
	release, err := s.locks.Acquire(ctx, repoRoot)
	if err != nil {
		return domain.Run{}, errorsx.Wrap(errorsx.CodeActiveRunExists, "active run exists for repository", errorsx.ExitUsage, err)
	}
	defer release()
	if run.Status != domain.RunStatusSucceeded && run.Status != domain.RunStatusFailed {
		return domain.Run{}, errorsx.Wrap(errorsx.CodeValidation, "run status cannot be applied", errorsx.ExitUsage, domain.ErrValidation)
	}
	if run.PostScanStatus != "clean" {
		return domain.Run{}, errorsx.Wrap(errorsx.CodePostScanBlocked, "postflight scan is not clean", errorsx.ExitSecurityBlocked, domain.ErrPostScanBlocked)
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
	if err := s.store.SetRunStatus(ctx, run.ID, domain.RunStatusApplying); err != nil {
		return domain.Run{}, err
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
		RunID: run.ID, RepoPath: repoRoot, PatchPath: run.PatchPath, Strategy: "git_apply_3way", BranchName: branchName, Status: "started", CreatedAt: s.clock.Now(),
	})
	if err != nil {
		return domain.Run{}, err
	}
	applyReq := domain.ApplyPatchRequest{RepoPath: repoRoot, PatchPath: run.PatchPath, CreateBranch: createBranch, BranchName: branchName, Reject: req.Reject, RejectDir: filepath.Join(run.RunDir, "rejects")}
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
		return domain.Run{}, err
	}
	if err := s.store.SetRunStatus(ctx, run.ID, domain.RunStatusApplied); err != nil {
		return domain.Run{}, err
	}
	applyFinished = true
	run.Status = domain.RunStatusApplied
	return run, nil
}

func suppressBestEffortError(error) {}
