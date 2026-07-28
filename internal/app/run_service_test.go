package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/agentfence/agentfence/internal/config"
	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/execx"
	"github.com/agentfence/agentfence/internal/policy"
	"github.com/agentfence/agentfence/internal/ports"
	"github.com/agentfence/agentfence/internal/state"
	"github.com/agentfence/agentfence/internal/testutil"
)

func TestRunServicePreflightBlockPreventsAgent(t *testing.T) {
	t.Parallel()
	runService, store, agent, _ := newTestRunService(t, domain.ScanResult{
		Status:   "blocked",
		Blocked:  true,
		Findings: []domain.Finding{{Severity: "high", RuleID: "generic-api-key", RedactedSecret: "****"}},
	})
	_, err := runService.Run(context.Background(), domain.RunRequest{RepoPath: "/repo", Agent: "fake", Task: "do it"})
	if err == nil {
		t.Fatalf("expected blocked error")
	}
	if agent.Called {
		t.Fatalf("agent was called despite preflight block")
	}
	if len(store.Runs) != 1 {
		t.Fatalf("run not persisted")
	}
}

func TestRunServiceSucceeded(t *testing.T) {
	t.Parallel()
	runService, store, _, _ := newTestRunService(t, domain.ScanResult{Status: "clean"})
	result, err := runService.Run(context.Background(), domain.RunRequest{RepoPath: "/repo", Agent: "fake", Task: "do it"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Status != domain.RunStatusSucceeded {
		t.Fatalf("status=%s", result.Status)
	}
	run, err := store.GetRun(context.Background(), result.RunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.TaskRedacted != persistedTask {
		t.Fatalf("task was persisted: %q", run.TaskRedacted)
	}
}

func TestRunServiceRejectsUnknownNetworkOverride(t *testing.T) {
	t.Parallel()
	runService, _, _, _ := newTestRunService(t, domain.ScanResult{Status: "clean"})
	_, err := runService.Run(context.Background(), domain.RunRequest{
		RepoPath: "/repo", Agent: "fake", Task: "do it", Network: "denny",
	})
	if err == nil {
		t.Fatalf("unknown network override was accepted")
	}
}

func TestRunOverridesAllowRequestedDirtyInputs(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultConfig()
	applyRunOverrides(&cfg, domain.RunRequest{IncludeDirty: true, IncludeUntracked: true})
	if cfg.Workspace.RequireCleanTree {
		t.Fatalf("explicit dirty input flags did not override clean-tree requirement")
	}
}

func TestSafeConfigSnapshotDropsSensitivePathsAndArguments(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultConfig()
	cfg.Workspace.StateDir = "/secret/state"
	cfg.Workspace.Exclude = []string{"build/**"}
	cfg.Agent.AuthMounts = []config.AuthMount{
		{HostPath: "/secret/token", SandboxPath: "/agent-home/token"},
	}
	adapter := cfg.Agent.Adapters["codex"]
	adapter.Command = filepath.Join("/secret", "bin", "codex")
	adapter.Args = []string{"--token", "raw-secret"}
	cfg.Agent.Adapters["codex"] = adapter
	snapshot := safeConfigSnapshot(cfg)
	encoded := fmt.Sprint(snapshot)
	for _, secret := range []string{"/secret/state", "/secret/token", "raw-secret"} {
		if strings.Contains(encoded, secret) {
			t.Fatalf("snapshot leaked %q: %s", secret, encoded)
		}
	}
	if !slices.Contains(snapshot.Workspace.Exclude, ".env") {
		t.Fatalf("snapshot omitted effective mandatory excludes: %#v", snapshot.Workspace.Exclude)
	}
}

func TestRunServiceBuildsControlsFromTargetConfig(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	configData := []byte(
		"version: 1\n" +
			"scan:\n  engines: [trufflehog]\n" +
			"sandbox:\n  mode: soft\n  network: allow\n  allow_soft_mode: true\n",
	)
	if err := os.WriteFile(filepath.Join(repoRoot, ".agentfence.yml"), configData, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	store := testutil.NewFakeStore()
	git := &testutil.FakeGit{RepoRoot: repoRoot}
	paths := state.Paths{RunsDir: filepath.Join(t.TempDir(), "runs")}
	var scannerEngines []string
	var sandboxMode string
	service, err := NewRunService(RunServiceDeps{
		Store:     store,
		Git:       git,
		Policy:    policy.NewPlanner(git),
		Workspace: &testutil.FakeWorkspace{PatchBytes: []byte("diff")},
		ScannerFactory: func(cfg config.ScanConfig) ([]ports.Scanner, error) {
			scannerEngines = append([]string{}, cfg.Engines...)
			return []ports.Scanner{&testutil.FakeScanner{Result: domain.ScanResult{Status: "clean"}}}, nil
		},
		SandboxFactory: func(cfg config.SandboxConfig) (ports.Sandbox, error) {
			sandboxMode = cfg.Mode
			return &testutil.FakeSandbox{
				Result: domain.SandboxRunResult{ExitCode: 0, StartedAt: time.Now(), FinishedAt: time.Now()},
			}, nil
		},
		Agents:   &testutil.FakeAgentRegistry{},
		Locks:    testutil.FakeLockManager{},
		Paths:    paths,
		Redactor: execx.NewRedactor(),
		Logger:   slog.Default(),
		Clock:    ports.SystemClock{},
	})
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	if _, err := service.Run(context.Background(), domain.RunRequest{
		RepoPath: repoRoot, Agent: "fake", Task: "task",
	}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(scannerEngines) != 1 || scannerEngines[0] != "trufflehog" || sandboxMode != "soft" {
		t.Fatalf("wrong controls: scanners=%v sandbox=%s", scannerEngines, sandboxMode)
	}
}

func TestRunServiceRegistersScannerRawSecrets(t *testing.T) {
	t.Parallel()
	rawSecret := "scanner-known-secret-value"
	runService, _, _, redactor := newTestRunService(t, domain.ScanResult{
		Status:     "findings",
		RawSecrets: []string{rawSecret},
	})
	_, err := runService.Run(context.Background(), domain.RunRequest{RepoPath: "/repo", Agent: "fake", Task: "do it"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	redacted := redactor.RedactString("token=" + rawSecret)
	if strings.Contains(redacted, rawSecret) {
		t.Fatalf("raw scanner secret was not registered")
	}
}

func TestRunServiceStopsAgentWhenWorkspaceLimitIsExceeded(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeStore()
	git := &testutil.FakeGit{RepoRoot: "/repo"}
	workspace := &testutil.FakeWorkspace{PatchBytes: []byte("diff")}
	validationCalls := 0
	workspace.ValidateFunc = func(ctx context.Context, shadowPath string) error {
		validationCalls++
		if validationCalls >= 2 {
			return domain.ErrPostScanBlocked
		}
		return nil
	}
	paths := state.Paths{
		StateDir: t.TempDir(),
		CacheDir: t.TempDir(),
		RunsDir:  t.TempDir(),
		LocksDir: t.TempDir(),
	}
	service, err := NewRunService(RunServiceDeps{
		Store: store, Git: git, Policy: policy.NewPlanner(git), Workspace: workspace,
		Scanners: []ports.Scanner{&testutil.FakeScanner{Result: domain.ScanResult{Status: "clean"}}},
		Sandbox:  blockingSandbox{}, Agents: &testutil.FakeAgentRegistry{}, Locks: testutil.FakeLockManager{},
		Paths: paths, Redactor: execx.NewRedactor(), Logger: slog.Default(), Clock: ports.SystemClock{},
	})
	if err != nil {
		t.Fatalf("service: %v", err)
	}

	result, err := service.Run(
		context.Background(),
		domain.RunRequest{RepoPath: "/repo", Agent: "fake", Task: "write too much"},
	)

	if !errors.Is(err, domain.ErrPostScanBlocked) {
		t.Fatalf("error = %v, want post-scan blocked", err)
	}
	if result.Status != domain.RunStatusBlockedPost {
		t.Fatalf("status = %s, want blocked post", result.Status)
	}
	run, getErr := store.GetRun(context.Background(), result.RunID)
	if getErr != nil {
		t.Fatalf("get run: %v", getErr)
	}
	if run.Status != domain.RunStatusBlockedPost {
		t.Fatalf("persisted status = %s", run.Status)
	}
}

func TestRunServicePassesSanitizedEnvToSandbox(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeStore()
	git := &testutil.FakeGit{RepoRoot: "/repo"}
	agent := &testutil.FakeAgentRegistry{
		Env: []string{"DATABASE_URL=postgres://real-user:real-pass@127.0.0.1/prod", "TERM=xterm"},
	}
	paths := state.Paths{StateDir: t.TempDir(), CacheDir: t.TempDir(), RunsDir: t.TempDir(), LocksDir: t.TempDir()}
	redactor := execx.NewRedactor()
	sandbox := &testutil.FakeSandbox{Result: domain.SandboxRunResult{
		ExitCode:   0,
		StartedAt:  time.Now(),
		FinishedAt: time.Now(),
	}}
	workspace := &testutil.FakeWorkspace{
		PatchBytes:        []byte("diff"),
		SanitizedEnv:      []string{"DATABASE_URL=postgres://agentfence:agentfence@127.0.0.1:1/agentfence?sslmode=disable"},
		IgnoredPatchPaths: []string{".env"},
	}
	service, err := NewRunService(RunServiceDeps{
		Store: store, Git: git, Policy: policy.NewPlanner(git), Workspace: workspace,
		Scanners: []ports.Scanner{&testutil.FakeScanner{Result: domain.ScanResult{Status: "clean"}}},
		Sandbox:  sandbox, Agents: agent, Locks: testutil.FakeLockManager{}, Paths: paths,
		Redactor: redactor, Logger: slog.Default(), Clock: ports.SystemClock{},
	})
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	_, err = service.Run(context.Background(), domain.RunRequest{RepoPath: "/repo", Agent: "fake", Task: "do it"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(sandbox.Request.Invocation.Env) != 2 {
		t.Fatalf("env=%v", sandbox.Request.Invocation.Env)
	}
	if sandbox.Request.Invocation.Env[0] != "TERM=xterm" {
		t.Fatalf("env=%v", sandbox.Request.Invocation.Env)
	}
	if sandbox.Request.Invocation.Env[1] != workspace.SanitizedEnv[0] {
		t.Fatalf("env=%v", sandbox.Request.Invocation.Env)
	}
	for _, value := range sandbox.Request.Invocation.Env {
		if strings.Contains(value, "real-user") {
			t.Fatalf("host env leaked: %v", sandbox.Request.Invocation.Env)
		}
	}
}

func newTestRunService(t *testing.T, scanResult domain.ScanResult) (*RunService, *testutil.FakeStore, *testutil.FakeAgentRegistry, *execx.Redactor) {
	t.Helper()
	store := testutil.NewFakeStore()
	git := &testutil.FakeGit{RepoRoot: "/repo"}
	agent := &testutil.FakeAgentRegistry{}
	paths := state.Paths{StateDir: t.TempDir(), CacheDir: t.TempDir(), RunsDir: t.TempDir(), LocksDir: t.TempDir()}
	redactor := execx.NewRedactor()
	service, err := NewRunService(RunServiceDeps{
		Store: store, Git: git, Policy: policy.NewPlanner(git), Workspace: &testutil.FakeWorkspace{PatchBytes: []byte("diff")},
		Scanners: []ports.Scanner{&testutil.FakeScanner{Result: scanResult}}, Sandbox: &testutil.FakeSandbox{Result: domain.SandboxRunResult{ExitCode: 0, StartedAt: time.Now(), FinishedAt: time.Now()}},
		Agents: agent, Locks: testutil.FakeLockManager{}, Paths: paths, Redactor: redactor, Logger: slog.Default(), Clock: ports.SystemClock{},
	})
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	return service, store, agent, redactor
}

type blockingSandbox struct{}

func (blockingSandbox) Run(
	ctx context.Context,
	req domain.SandboxRunRequest,
) (domain.SandboxRunResult, error) {
	<-ctx.Done()
	return domain.SandboxRunResult{}, ctx.Err()
}
