package app

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentfence/agentfence/internal/config"
	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/errorsx"
	"github.com/agentfence/agentfence/internal/execx"
	"github.com/agentfence/agentfence/internal/ports"
	"github.com/agentfence/agentfence/internal/scanner"
	"github.com/agentfence/agentfence/internal/state"
	"github.com/oklog/ulid/v2"
)

type RunService struct {
	store          ports.Store
	git            ports.GitRunner
	policy         ports.PolicyPlanner
	workspace      ports.WorkspaceManager
	scanners       []ports.Scanner
	scannerFactory ScannerFactory
	sandbox        ports.Sandbox
	sandboxFactory SandboxFactory
	agents         ports.AgentRegistry
	locks          ports.LockManager
	paths          state.Paths
	redactor       *execx.Redactor
	logger         *slog.Logger
	clock          ports.Clock
}

const (
	postRunGracePeriod   = 30 * time.Second
	persistenceTimeout   = 10 * time.Second
	workspaceWatchPeriod = 50 * time.Millisecond
	persistedTask        = "[redacted]"
)

type ScannerFactory func(config.ScanConfig) ([]ports.Scanner, error)

type SandboxFactory func(config.SandboxConfig) (ports.Sandbox, error)

type RunServiceDeps struct {
	Store          ports.Store
	Git            ports.GitRunner
	Policy         ports.PolicyPlanner
	Workspace      ports.WorkspaceManager
	Scanners       []ports.Scanner
	ScannerFactory ScannerFactory
	Sandbox        ports.Sandbox
	SandboxFactory SandboxFactory
	Agents         ports.AgentRegistry
	Locks          ports.LockManager
	Paths          state.Paths
	Redactor       *execx.Redactor
	Logger         *slog.Logger
	Clock          ports.Clock
}

type configurableAgentRegistry interface {
	WithConfig(cfg config.AgentConfig) ports.AgentRegistry
}

type runContextStore interface {
	SetRunContext(ctx context.Context, runID string, repoHead string, baseRef string, networkMode string, timeoutSeconds int) error
}

func NewRunService(deps RunServiceDeps) (*RunService, error) {
	scannersMissing := len(deps.Scanners) == 0 && deps.ScannerFactory == nil
	sandboxMissing := deps.Sandbox == nil && deps.SandboxFactory == nil
	if deps.Store == nil || deps.Git == nil || deps.Policy == nil || deps.Workspace == nil || sandboxMissing ||
		deps.Agents == nil || deps.Locks == nil || deps.Redactor == nil || deps.Logger == nil || deps.Clock == nil ||
		scannersMissing {
		return nil, errorsx.Wrap(errorsx.CodeInternal, "run service dependency missing", errorsx.ExitInternal, nil)
	}
	return &RunService{
		store: deps.Store, git: deps.Git, policy: deps.Policy, workspace: deps.Workspace,
		scanners: deps.Scanners, scannerFactory: deps.ScannerFactory,
		sandbox: deps.Sandbox, sandboxFactory: deps.SandboxFactory, agents: deps.Agents, locks: deps.Locks,
		paths: deps.Paths, redactor: deps.Redactor, logger: deps.Logger, clock: deps.Clock,
	}, nil
}

func (s *RunService) Run(ctx context.Context, req domain.RunRequest) (domain.RunResult, error) {
	if strings.TrimSpace(req.Task) == "" {
		return domain.RunResult{}, errorsx.Wrap(errorsx.CodeValidation, "task is required", errorsx.ExitUsage, domain.ErrValidation)
	}
	startDir := req.RepoPath
	if startDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return domain.RunResult{}, fmt.Errorf("get working directory: %w", err)
		}
		startDir = cwd
	}
	repoRoot, err := s.git.FindRepoRoot(ctx, startDir)
	if err != nil {
		return domain.RunResult{}, errorsx.Wrap(errorsx.CodeNotGitRepo, "not a git repository", errorsx.ExitUsage, err)
	}
	cfg, err := config.LoadForRepo(ctx, repoRoot, req.ConfigPath)
	if err != nil {
		return domain.RunResult{}, err
	}
	applyRunOverrides(&cfg, req)
	if err := config.Validate(cfg); err != nil {
		return domain.RunResult{}, err
	}
	scanners, err := s.scannersForConfig(cfg.Scan)
	if err != nil {
		return domain.RunResult{}, err
	}
	sandboxAdapter, err := s.sandboxForConfig(cfg.Sandbox)
	if err != nil {
		return domain.RunResult{}, err
	}
	timeoutSeconds := req.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 3600
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	runID := req.RunID
	now := s.clock.Now()
	if runID == "" {
		generatedID, err := newULID(now)
		if err != nil {
			return domain.RunResult{}, errorsx.Wrap(errorsx.CodeInternal, "generate run id failed", errorsx.ExitInternal, err)
		}
		runID = generatedID
		run := domain.Run{
			ID: runID, RepoPath: repoRoot, RepoHead: "", BaseRef: "",
			RunDir: s.paths.RunDir(runID), ShadowPath: filepath.Join(s.paths.RunDir(runID), "shadow"),
			MetadataPath: filepath.Join(s.paths.RunDir(runID), "shadow_metadata.json"), PatchPath: filepath.Join(s.paths.RunDir(runID), "changes.patch"),
			AgentName: req.Agent, TaskRedacted: persistedTask, Status: domain.RunStatusCreated,
			PreScanStatus: "pending", PostScanStatus: "pending", NetworkMode: cfg.Sandbox.Network, IsolationLevel: "",
			TimeoutSeconds: timeoutSeconds, CreatedAt: now, UpdatedAt: now,
		}
		if err := s.store.CreateRun(ctx, run); err != nil {
			return domain.RunResult{}, fmt.Errorf("create run row: %w", err)
		}
	} else {
		if _, err := ulid.Parse(runID); err != nil {
			return domain.RunResult{}, errorsx.Wrap(errorsx.CodeValidation, "invalid run id", errorsx.ExitUsage, err)
		}
		existing, err := s.store.GetRun(ctx, runID)
		if err != nil {
			return domain.RunResult{}, mapRunLoadError(err)
		}
		if existing.Status != domain.RunStatusCreated {
			return domain.RunResult{}, errorsx.Wrap(errorsx.CodeValidation, "run is not in created status", errorsx.ExitUsage, nil)
		}
	}
	runDir := s.paths.RunDir(runID)
	patchPath := filepath.Join(runDir, "changes.patch")
	terminal := false
	defer func() {
		if !terminal {
			status, code, message := terminalFailure(runCtx.Err())
			finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), persistenceTimeout)
			defer finishCancel()
			if err := s.finish(finishCtx, runID, status, nil, "", code, message); err != nil {
				s.logger.ErrorContext(finishCtx, "finalize incomplete run failed", slog.String("run_id", runID), slog.Any("error", err))
			}
		}
	}()

	if err := s.store.SetRunStatus(ctx, runID, domain.RunStatusPreparing); err != nil {
		return domain.RunResult{}, err
	}
	release, err := s.locks.Acquire(runCtx, repoRoot)
	if err != nil {
		return domain.RunResult{}, errorsx.Wrap(errorsx.CodeActiveRunExists, "active run exists for repository", errorsx.ExitUsage, err)
	}
	defer release()

	if cfg.Workspace.RequireCleanTree {
		status, err := s.git.StatusPorcelain(runCtx, repoRoot)
		if err != nil {
			return domain.RunResult{}, fmt.Errorf("check clean tree: %w", err)
		}
		if len(status) > 0 {
			if finishErr := s.finishDurably(ctx, runID, domain.RunStatusFailed, nil, "", errorsx.CodeRepoDirty, "repository is dirty"); finishErr != nil {
				return domain.RunResult{}, finishErr
			}
			terminal = true
			return domain.RunResult{}, errorsx.Wrap(errorsx.CodeRepoDirty, "repository is dirty", errorsx.ExitUsage, domain.ErrRepoDirty)
		}
	}
	repoHead, err := s.git.HeadSHA(runCtx, repoRoot)
	if err != nil {
		return domain.RunResult{}, fmt.Errorf("read repo head: %w", err)
	}
	baseRef, err := s.git.BaseRef(runCtx, repoRoot)
	if err != nil {
		return domain.RunResult{}, fmt.Errorf("read base ref: %w", err)
	}
	contextStore, ok := s.store.(runContextStore)
	if !ok {
		return domain.RunResult{}, errorsx.Wrap(errorsx.CodeInternal, "store cannot update run context", errorsx.ExitInternal, nil)
	}
	if err := contextStore.SetRunContext(runCtx, runID, repoHead, baseRef, cfg.Sandbox.Network, timeoutSeconds); err != nil {
		return domain.RunResult{}, err
	}
	plan, err := s.policy.BuildExposurePlan(runCtx, domain.ExposurePlanRequest{
		RepoPath: repoRoot, RepoHead: repoHead, BaseRef: baseRef, IncludeUntracked: req.IncludeUntracked, IncludeDirty: req.IncludeDirty, Config: cfg,
	})
	if err != nil {
		return domain.RunResult{}, fmt.Errorf("build exposure plan: %w", err)
	}
	cfgJSON, err := json.Marshal(safeConfigSnapshot(cfg))
	if err != nil {
		return domain.RunResult{}, fmt.Errorf("marshal config snapshot: %w", err)
	}
	policyJSON, err := json.Marshal(plan)
	if err != nil {
		return domain.RunResult{}, fmt.Errorf("marshal policy snapshot: %w", err)
	}
	if err := s.store.SaveConfigSnapshot(runCtx, runID, string(cfgJSON), string(policyJSON)); err != nil {
		return domain.RunResult{}, err
	}
	workspaceResult, err := s.workspace.Create(runCtx, domain.CreateWorkspaceRequest{
		RunID: runID, RepoRoot: repoRoot, RunDir: runDir, ExposurePlan: plan, RepoHead: repoHead, BaseRef: baseRef,
		SanitizedEnv: cfg.Agent.SanitizedEnv,
	})
	if err != nil {
		return domain.RunResult{}, fmt.Errorf("create shadow workspace: %w", err)
	}
	if err := s.store.SetRunStatus(runCtx, runID, domain.RunStatusScanning); err != nil {
		return domain.RunResult{}, err
	}
	preScan, err := s.scanPhase(
		runCtx,
		cfg,
		scanners,
		runID,
		domain.FindingPhasePreflight,
		workspaceResult.ShadowPath,
		runDir,
	)
	if err != nil {
		return domain.RunResult{}, err
	}
	if err := s.store.SetScanStatus(runCtx, runID, preScan.Status, "pending"); err != nil {
		return domain.RunResult{}, err
	}
	if preScan.Blocked {
		if err := s.finishDurably(ctx, runID, domain.RunStatusBlocked, nil, "", errorsx.CodeScanBlocked, "preflight scan blocked run"); err != nil {
			return domain.RunResult{}, err
		}
		terminal = true
		return domain.RunResult{RunID: runID, Status: domain.RunStatusBlocked}, errorsx.Wrap(errorsx.CodeScanBlocked, "preflight scan blocked run", errorsx.ExitSecurityBlocked, domain.ErrScanBlocked)
	}
	registry := s.agents
	if configurable, ok := s.agents.(configurableAgentRegistry); ok {
		registry = configurable.WithConfig(cfg.Agent)
	}
	invocation, err := registry.BuildInvocation(runCtx, req.Agent, req.Task, req.AgentArgs, req.AdapterOverride)
	if err != nil {
		return domain.RunResult{}, errorsx.Wrap(errorsx.CodeDependencyMissing, "agent command unavailable", errorsx.ExitDependencyMissing, err)
	}
	invocation.Env = mergeInvocationEnv(invocation.Env, workspaceResult.SanitizedEnv)
	stdout, stderr, flush, err := s.openRedactedLogs(workspaceResult.LogsDir)
	if err != nil {
		return domain.RunResult{}, err
	}
	defer flush()
	if cfg.Sandbox.Network == "allow" {
		if err := s.event(runCtx, runID, "warn", "network_allowed", "network access allowed", "{}"); err != nil {
			return domain.RunResult{}, err
		}
	}
	if err := s.store.SetRunStarted(runCtx, runID); err != nil {
		return domain.RunResult{}, err
	}
	if err := s.store.SetRunStatus(runCtx, runID, domain.RunStatusRunning); err != nil {
		return domain.RunResult{}, err
	}
	runResult, runErr := s.runSandboxWithWorkspaceWatch(runCtx, sandboxAdapter, domain.SandboxRunRequest{
		Invocation: invocation, ShadowPath: workspaceResult.ShadowPath, LogsDir: workspaceResult.LogsDir, NetworkMode: cfg.Sandbox.Network,
		TimeoutSeconds: timeoutSeconds, WritablePaths: cfg.Sandbox.WritablePaths, AuthMounts: cfg.Agent.AuthMounts, EnvAllowlist: cfg.Agent.EnvAllowlist,
		Stdout: stdout, Stderr: stderr, AllowSoftMode: cfg.Sandbox.AllowSoftMode || req.AllowSoftMode,
		ReadonlySysPaths: cfg.Sandbox.ReadonlySystemPaths, TmpfsPaths: cfg.Sandbox.TmpfsPaths,
	})
	postCtx := runCtx
	if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
		var postCancel context.CancelFunc
		postCtx, postCancel = context.WithTimeout(context.WithoutCancel(ctx), postRunGracePeriod)
		defer postCancel()
	}
	workspaceViolation := errors.Is(runErr, domain.ErrPostScanBlocked)
	validationErr := s.workspace.ValidatePostRunWorkspace(postCtx, workspaceResult.ShadowPath)
	if validationErr != nil || workspaceViolation {
		if validationErr == nil {
			validationErr = runErr
		}
		if finishErr := s.finishDurably(ctx, runID, domain.RunStatusBlockedPost, &runResult.ExitCode, runResult.IsolationLevel, errorsx.CodePostScanBlocked, "unsafe post-run workspace"); finishErr != nil {
			return domain.RunResult{}, finishErr
		}
		terminal = true
		return domain.RunResult{RunID: runID, Status: domain.RunStatusBlockedPost, PatchPath: patchPath}, errorsx.Wrap(errorsx.CodePostScanBlocked, "unsafe post-run workspace", errorsx.ExitSecurityBlocked, validationErr)
	}
	if err := s.store.SetRunStatus(postCtx, runID, domain.RunStatusPostScan); err != nil {
		return domain.RunResult{}, err
	}
	postScan, err := s.scanPhase(
		postCtx,
		cfg,
		scanners,
		runID,
		domain.FindingPhasePostflight,
		workspaceResult.ShadowPath,
		runDir,
	)
	if err != nil {
		return domain.RunResult{}, err
	}
	if err := s.workspace.GeneratePatch(postCtx, domain.GeneratePatchRequest{
		RunDir: runDir, ShadowPath: workspaceResult.ShadowPath, TrustedGitDir: workspaceResult.TrustedGitDir,
		PatchPath:         patchPath,
		IgnoredPatchPaths: workspaceResult.IgnoredPatchPaths,
	}); err != nil {
		return domain.RunResult{}, err
	}
	patchScan, err := s.scanPhase(postCtx, cfg, scanners, runID, domain.FindingPhasePatch, patchPath, runDir)
	if err != nil {
		return domain.RunResult{}, err
	}
	postStatus := postScan.Status
	if patchScan.Blocked {
		postStatus = "blocked"
	}
	if postStatus == "clean" && patchScan.Status != "" && patchScan.Status != "clean" {
		postStatus = patchScan.Status
	}
	if err := s.store.SetScanStatus(postCtx, runID, preScan.Status, postStatus); err != nil {
		return domain.RunResult{}, err
	}
	if postScan.Blocked || patchScan.Blocked {
		if err := s.finishDurably(ctx, runID, domain.RunStatusBlockedPost, &runResult.ExitCode, runResult.IsolationLevel, errorsx.CodePostScanBlocked, "postflight scan blocked apply"); err != nil {
			return domain.RunResult{}, err
		}
		terminal = true
		return domain.RunResult{RunID: runID, Status: domain.RunStatusBlockedPost, PatchPath: patchPath}, errorsx.Wrap(errorsx.CodePostScanBlocked, "postflight scan blocked apply", errorsx.ExitSecurityBlocked, domain.ErrPostScanBlocked)
	}
	status := domain.RunStatusSucceeded
	code := ""
	message := ""
	var resultErr error
	if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
		status = domain.RunStatusCancelled
		code = errorsx.CodeCancelled
		message = "run cancelled"
		exitCode := errorsx.ExitCancelled
		if errors.Is(runErr, context.DeadlineExceeded) {
			status = domain.RunStatusTimedOut
			code = errorsx.CodeTimeout
			message = "run timed out"
			exitCode = errorsx.ExitTimeout
		}
		resultErr = errorsx.Wrap(code, message, exitCode, runErr)
	} else if runErr != nil || runResult.ExitCode != 0 {
		status = domain.RunStatusFailed
		code = errorsx.CodeAgentFailed
		message = "agent failed"
		resultErr = errorsx.Wrap(code, message, errorsx.ExitInternal, runErr)
	}
	if err := s.finishDurably(ctx, runID, status, &runResult.ExitCode, runResult.IsolationLevel, code, message); err != nil {
		return domain.RunResult{}, err
	}
	terminal = true
	return domain.RunResult{RunID: runID, Status: status, PatchPath: patchPath}, resultErr
}

func (s *RunService) runSandboxWithWorkspaceWatch(
	ctx context.Context,
	sandboxAdapter ports.Sandbox,
	req domain.SandboxRunRequest,
) (domain.SandboxRunResult, error) {
	sandboxCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	watchResult := make(chan error, 1)
	go func() {
		err := s.watchWorkspace(sandboxCtx, req.ShadowPath)
		if err != nil {
			cancel()
		}
		watchResult <- err
	}()
	result, runErr := sandboxAdapter.Run(sandboxCtx, req)
	cancel()
	watchErr := <-watchResult
	if watchErr != nil {
		return result, watchErr
	}
	return result, runErr
}

func (s *RunService) watchWorkspace(ctx context.Context, shadowPath string) error {
	ticker := time.NewTicker(workspaceWatchPeriod)
	defer ticker.Stop()
	for {
		err := s.workspace.ValidatePostRunWorkspace(ctx, shadowPath)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (s *RunService) scanPhase(
	ctx context.Context,
	cfg config.Config,
	scanners []ports.Scanner,
	runID string,
	phase domain.FindingPhase,
	target string,
	runDir string,
) (domain.ScanResult, error) {
	if !cfg.Scan.Enabled {
		if err := s.event(ctx, runID, "warn", "scan_disabled", "scanner disabled", "{}"); err != nil {
			return domain.ScanResult{}, err
		}
		return domain.ScanResult{Status: "clean"}, nil
	}
	result, err := scanner.ScanAll(ctx, scanners, domain.ScanRequest{
		RunID: runID, Phase: phase, TargetPath: target, RunDir: runDir, TimeoutSeconds: cfg.Scan.TimeoutSeconds,
	}, cfg.Scan.SeverityBlocklist, cfg.Scan.FailOnFindings)
	if err != nil {
		return domain.ScanResult{}, err
	}
	for _, secret := range result.RawSecrets {
		s.redactor.RegisterSecret(secret)
	}
	if err := s.store.InsertFindings(ctx, result.Findings); err != nil {
		return domain.ScanResult{}, err
	}
	return result, nil
}

func (s *RunService) openRedactedLogs(logsDir string) (*execx.RedactingWriter, *execx.RedactingWriter, func(), error) {
	if err := os.MkdirAll(logsDir, 0o700); err != nil {
		return nil, nil, nil, fmt.Errorf("create logs directory: %w", err)
	}
	stdoutFile, err := os.OpenFile(filepath.Join(logsDir, "stdout.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create stdout log: %w", err)
	}
	stderrFile, err := os.OpenFile(filepath.Join(logsDir, "stderr.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		closeErr := stdoutFile.Close()
		if closeErr != nil {
			return nil, nil, nil, fmt.Errorf("create stderr log: %w; close stdout log: %w", err, closeErr)
		}
		return nil, nil, nil, fmt.Errorf("create stderr log: %w", err)
	}
	stdout := execx.NewRedactingWriter(stdoutFile, s.redactor)
	stderr := execx.NewRedactingWriter(stderrFile, s.redactor)
	flush := func() {
		if err := stdout.Flush(); err != nil {
			s.logger.Error("flush stdout log failed", slog.Any("error", err))
		}
		if err := stderr.Flush(); err != nil {
			s.logger.Error("flush stderr log failed", slog.Any("error", err))
		}
		if err := stdoutFile.Close(); err != nil {
			s.logger.Error("close stdout log failed", slog.Any("error", err))
		}
		if err := stderrFile.Close(); err != nil {
			s.logger.Error("close stderr log failed", slog.Any("error", err))
		}
	}
	return stdout, stderr, flush, nil
}

func (s *RunService) finish(ctx context.Context, runID string, status domain.RunStatus, exitCode *int, isolationLevel string, code string, message string) error {
	return s.store.SetRunFinished(ctx, domain.RunFinishResult{
		RunID: runID, Status: status, ExitCode: exitCode, IsolationLevel: isolationLevel, ErrorCode: code, ErrorMessage: message, FinishedAt: s.clock.Now(),
	})
}

func (s *RunService) finishDurably(
	ctx context.Context,
	runID string,
	status domain.RunStatus,
	exitCode *int,
	isolationLevel string,
	code string,
	message string,
) error {
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), persistenceTimeout)
	defer cancel()
	return s.finish(finishCtx, runID, status, exitCode, isolationLevel, code, message)
}

func (s *RunService) event(ctx context.Context, runID string, level string, eventType string, message string, dataJSON string) error {
	return s.store.InsertEvent(ctx, domain.RunEvent{RunID: runID, Ts: s.clock.Now(), Level: level, EventType: eventType, Message: message, DataJSON: dataJSON})
}

func applyRunOverrides(cfg *config.Config, req domain.RunRequest) {
	if req.Network != "" {
		cfg.Sandbox.Network = req.Network
	}
	if req.AllowSoftMode {
		cfg.Sandbox.AllowSoftMode = true
	}
	if req.IncludeDirty {
		cfg.Workspace.IncludeDirty = true
		cfg.Workspace.RequireCleanTree = false
	}
	if req.IncludeUntracked {
		cfg.Workspace.IncludeUntracked = true
		cfg.Workspace.RequireCleanTree = false
	}
}

func (s *RunService) scannersForConfig(cfg config.ScanConfig) ([]ports.Scanner, error) {
	if s.scannerFactory != nil {
		return s.scannerFactory(cfg)
	}
	return s.scanners, nil
}

func (s *RunService) sandboxForConfig(cfg config.SandboxConfig) (ports.Sandbox, error) {
	if s.sandboxFactory != nil {
		return s.sandboxFactory(cfg)
	}
	return s.sandbox, nil
}

func safeConfigSnapshot(cfg config.Config) config.Config {
	snapshot := cfg
	snapshot.Workspace.Exclude = appendUniqueStrings(
		snapshot.Workspace.Exclude,
		config.MandatoryExcludes(),
	)
	snapshot.Workspace.StateDir = ""
	snapshot.Workspace.CacheDir = ""
	snapshot.Agent.AuthMounts = nil
	snapshot.Daemon.SocketPath = ""
	snapshot.Daemon.ListenAddress = ""
	snapshot.Daemon.TokenFile = ""
	snapshot.Agent.Adapters = make(map[string]config.AgentAdapterConfig, len(cfg.Agent.Adapters))
	for name, adapter := range cfg.Agent.Adapters {
		snapshot.Agent.Adapters[name] = config.AgentAdapterConfig{
			Command:  filepath.Base(adapter.Command),
			TaskMode: adapter.TaskMode,
		}
	}
	return snapshot
}

func appendUniqueStrings(values []string, additions []string) []string {
	result := append([]string{}, values...)
	seen := make(map[string]struct{}, len(result)+len(additions))
	for _, value := range result {
		seen[value] = struct{}{}
	}
	for _, value := range additions {
		if _, exists := seen[value]; exists {
			continue
		}
		result = append(result, value)
		seen[value] = struct{}{}
	}
	return result
}

func terminalFailure(err error) (domain.RunStatus, string, string) {
	if errors.Is(err, context.DeadlineExceeded) {
		return domain.RunStatusTimedOut, errorsx.CodeTimeout, "run timed out"
	}
	if errors.Is(err, context.Canceled) {
		return domain.RunStatusCancelled, errorsx.CodeCancelled, "run cancelled"
	}
	return domain.RunStatusFailed, errorsx.CodeInternal, "run did not complete"
}

func mergeInvocationEnv(base []string, sanitized []string) []string {
	if len(sanitized) == 0 {
		return base
	}
	sanitizedKeys := map[string]struct{}{}
	for _, item := range sanitized {
		key, _, ok := strings.Cut(item, "=")
		if ok && key != "" {
			sanitizedKeys[key] = struct{}{}
		}
	}
	result := make([]string, 0, len(base)+len(sanitized))
	for _, item := range base {
		key, _, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		if _, exists := sanitizedKeys[key]; exists {
			continue
		}
		result = append(result, item)
	}
	result = append(result, sanitized...)
	return result
}

func newULID(now time.Time) (string, error) {
	entropy := ulid.Monotonic(rand.Reader, 0)
	id, err := ulid.New(ulid.Timestamp(now), entropy)
	if err != nil {
		return "", fmt.Errorf("generate run id: %w", err)
	}
	return id.String(), nil
}

func mapRunLoadError(err error) error {
	if errors.Is(err, domain.ErrRunNotFound) {
		return errorsx.Wrap(errorsx.CodeRunNotFound, "run not found", errorsx.ExitNotFound, err)
	}
	return err
}
