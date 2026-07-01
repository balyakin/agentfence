package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentfence/agentfence/internal/config"
	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/errorsx"
	"github.com/agentfence/agentfence/internal/ports"
	"github.com/agentfence/agentfence/internal/scanner"
	"github.com/agentfence/agentfence/internal/state"
)

type ScanService struct {
	store     ports.Store
	git       ports.GitRunner
	policy    ports.PolicyPlanner
	workspace ports.WorkspaceManager
	scanners  []ports.Scanner
	paths     state.Paths
	clock     ports.Clock
}

type ScanServiceDeps struct {
	Store     ports.Store
	Git       ports.GitRunner
	Policy    ports.PolicyPlanner
	Workspace ports.WorkspaceManager
	Scanners  []ports.Scanner
	Paths     state.Paths
	Clock     ports.Clock
}

type ScanOnlyRequest struct {
	RepoPath         string
	ConfigPath       string
	IncludeUntracked bool
	IncludeDirty     bool
	RunID            string
}

func NewScanService(deps ScanServiceDeps) (*ScanService, error) {
	if deps.Git == nil || deps.Policy == nil || deps.Workspace == nil || deps.Clock == nil || len(deps.Scanners) == 0 {
		return nil, errorsx.Wrap(errorsx.CodeInternal, "scan service dependency missing", errorsx.ExitInternal, nil)
	}
	return &ScanService{store: deps.Store, git: deps.Git, policy: deps.Policy, workspace: deps.Workspace, scanners: deps.Scanners, paths: deps.Paths, clock: deps.Clock}, nil
}

func (s *ScanService) Scan(ctx context.Context, req ScanOnlyRequest) (result domain.ScanResult, err error) {
	start := req.RepoPath
	if start == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return domain.ScanResult{}, fmt.Errorf("get working directory: %w", err)
		}
		start = cwd
	}
	repoRoot, err := s.git.FindRepoRoot(ctx, start)
	if err != nil {
		return domain.ScanResult{}, err
	}
	cfg, err := config.LoadForRepo(ctx, repoRoot, req.ConfigPath)
	if err != nil {
		return domain.ScanResult{}, err
	}
	plan, err := s.policy.BuildExposurePlan(ctx, domain.ExposurePlanRequest{
		RepoPath: repoRoot, IncludeUntracked: req.IncludeUntracked, IncludeDirty: req.IncludeDirty, Config: cfg,
	})
	if err != nil {
		return domain.ScanResult{}, err
	}
	runID := req.RunID
	if runID == "" {
		runID = "scan-" + strings.ReplaceAll(s.clock.Now().Format("20060102150405.000000000"), ".", "")
	}
	runDir := filepath.Join(s.paths.RunsDir, runID)
	if req.RunID == "" {
		defer func() {
			if cleanupErr := os.RemoveAll(runDir); cleanupErr != nil && err == nil {
				err = fmt.Errorf("cleanup scan workspace: %w", cleanupErr)
			}
		}()
	}
	workspaceResult, err := s.workspace.Create(ctx, domain.CreateWorkspaceRequest{RunID: runID, RepoRoot: repoRoot, RunDir: runDir, ExposurePlan: plan})
	if err != nil {
		return domain.ScanResult{}, err
	}
	if !cfg.Scan.Enabled {
		if req.RunID != "" && s.store != nil {
			eventErr := s.store.InsertEvent(ctx, domain.RunEvent{
				RunID: req.RunID, Ts: s.clock.Now(), Level: "warn", EventType: "scan_disabled", Message: "scanner disabled", DataJSON: "{}",
			})
			if eventErr != nil {
				return domain.ScanResult{}, eventErr
			}
		}
		return domain.ScanResult{Status: "clean"}, nil
	}
	result, err = scanner.ScanAll(ctx, s.scanners, domain.ScanRequest{RunID: req.RunID, Phase: domain.FindingPhasePreflight, TargetPath: workspaceResult.ShadowPath, RunDir: runDir, TimeoutSeconds: cfg.Scan.TimeoutSeconds}, cfg.Scan.SeverityBlocklist)
	if err != nil {
		return domain.ScanResult{}, err
	}
	if req.RunID != "" && s.store != nil {
		if err := s.store.InsertFindings(ctx, result.Findings); err != nil {
			return domain.ScanResult{}, err
		}
	}
	return result, nil
}
