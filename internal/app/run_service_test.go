package app

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

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
	runService, _, _, _ := newTestRunService(t, domain.ScanResult{Status: "clean"})
	result, err := runService.Run(context.Background(), domain.RunRequest{RepoPath: "/repo", Agent: "fake", Task: "do it"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Status != domain.RunStatusSucceeded {
		t.Fatalf("status=%s", result.Status)
	}
}

func TestRunServiceRegistersScannerRawSecrets(t *testing.T) {
	t.Parallel()
	rawSecret := "scanner-known-secret-value"
	runService, _, _, redactor := newTestRunService(t, domain.ScanResult{
		Status:     "findings",
		RawSecrets: []string{rawSecret},
		Findings: []domain.Finding{
			{Severity: "low", RuleID: "generic-api-key", RedactedSecret: domain.RedactSecret(rawSecret)},
		},
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
