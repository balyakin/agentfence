package leaklab

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

const fakeAgentSandboxPath = "/agent-home/fake-agent.sh"

func TestLeakLab(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("leaklab hard-mode requires Linux")
	}
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bwrap not installed")
	}
	if _, err := exec.LookPath("gitleaks"); err != nil {
		t.Skip("gitleaks not installed")
	}
	bin := buildAgentFence(t)

	cleanRepo := createLeakLabRepo(t)
	cleanEnv := leakLabEnv(t, "")
	runOut, runErr := runAgentFence(
		t,
		cleanRepo,
		cleanEnv,
		bin,
		"run",
		"generic",
		"--command",
		"/bin/sh",
		"--agent-arg",
		fakeAgentSandboxPath,
		"--task",
		"probe",
		"--timeout",
		"30",
	)
	if runErr != nil {
		t.Fatalf("clean run failed: %v\n%s", runErr, runOut)
	}
	runID := parseRunID(t, runOut)
	diffOut, diffErr := runAgentFence(t, cleanRepo, cleanEnv, bin, "diff", "latest")
	if diffErr != nil {
		t.Fatalf("diff failed: %v\n%s", diffErr, diffOut)
	}
	if !strings.Contains(diffOut, "agentfence_fake_change.txt") || !strings.Contains(diffOut, "agentfence fake change") {
		t.Fatalf("diff missing fake change for run %s:\n%s", runID, diffOut)
	}
	if applyOut, err := runAgentFence(t, cleanRepo, cleanEnv, bin, "apply", "latest", "--branch"); err != nil {
		t.Fatalf("apply clean patch failed: %v\n%s", err, applyOut)
	}
	if _, err := os.Stat(filepath.Join(cleanRepo, "agentfence_fake_change.txt")); err != nil {
		t.Fatalf("applied fake change missing: %v", err)
	}

	secretRepo := createLeakLabRepo(t)
	secretEnv := leakLabEnv(t, "1")
	secretOut, secretErr := runAgentFence(
		t,
		secretRepo,
		secretEnv,
		bin,
		"run",
		"generic",
		"--command",
		"/bin/sh",
		"--agent-arg",
		fakeAgentSandboxPath,
		"--task",
		"probe",
		"--timeout",
		"30",
	)
	if secretErr == nil {
		t.Fatalf("secret injection run unexpectedly succeeded:\n%s", secretOut)
	}
	if !strings.Contains(secretOut, "blocked_post") && !strings.Contains(secretOut, "AF_POST_SCAN_BLOCKED") {
		t.Fatalf("secret injection did not block postflight:\n%s", secretOut)
	}
	applyOut, applyErr := runAgentFence(t, secretRepo, secretEnv, bin, "apply", "latest", "--branch")
	if applyErr == nil {
		t.Fatalf("apply unexpectedly succeeded for blocked_post run:\n%s", applyOut)
	}
	if !strings.Contains(applyOut, "AF_POST_SCAN_BLOCKED") {
		t.Fatalf("blocked apply did not report post scan block:\n%s", applyOut)
	}
}

func buildAgentFence(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "agentfence")
	cmd := exec.Command("go", "build", "-trimpath", "-o", bin, "../cmd/agentfence")
	cmd.Dir = "."
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build agentfence: %v\n%s", err, output)
	}
	return bin
}

func createLeakLabRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init")
	git(t, repo, "config", "user.email", "leaklab@example.invalid")
	git(t, repo, "config", "user.name", "Leak Lab")
	if err := os.WriteFile(filepath.Join(repo, "history.txt"), []byte("SOURCE_HISTORY_SECRET\n"), 0o600); err != nil {
		t.Fatalf("write history secret: %v", err)
	}
	git(t, repo, "add", "history.txt")
	git(t, repo, "commit", "-m", "history secret")
	git(t, repo, "rm", "history.txt")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("leaklab fixture\n"), 0o600); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".env"), []byte("LOCAL_ENV_SECRET=should-not-copy\n"), 0o600); err != nil {
		t.Fatalf("write env: %v", err)
	}
	configData := fmt.Sprintf(
		"version: 1\nagent:\n  env_allowlist:\n    - AF_INJECT_SECRET\n  auth_mounts:\n    - host_path: %q\n      sandbox_path: %s\n",
		fakeAgentPath(t),
		fakeAgentSandboxPath,
	)
	if err := os.WriteFile(filepath.Join(repo, ".agentfence.yml"), []byte(configData), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	git(t, repo, "add", "README.md", ".env", ".agentfence.yml")
	git(t, repo, "commit", "-m", "clean fixture")
	return repo
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func leakLabEnv(t *testing.T, injectSecret string) []string {
	t.Helper()
	stateDir := filepath.Join(t.TempDir(), "state")
	cacheDir := filepath.Join(t.TempDir(), "cache")
	env := append(os.Environ(), "XDG_STATE_HOME="+stateDir, "XDG_CACHE_HOME="+cacheDir)
	if injectSecret != "" {
		env = append(env, "AF_INJECT_SECRET="+injectSecret)
	}
	return env
}

func fakeAgentPath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs("fake-agent.sh")
	if err != nil {
		t.Fatalf("fake agent path: %v", err)
	}
	return path
}

func runAgentFence(t *testing.T, dir string, env []string, bin string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = env
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	if err != nil {
		appendRunLogs(&output, env)
	}
	return output.String(), err
}

func appendRunLogs(output *bytes.Buffer, env []string) {
	cacheDir := ""
	for _, value := range env {
		if path, ok := strings.CutPrefix(value, "XDG_CACHE_HOME="); ok {
			cacheDir = path
			break
		}
	}
	if cacheDir == "" {
		return
	}
	paths, _ := filepath.Glob(filepath.Join(cacheDir, "agentfence", "runs", "*", "logs", "*.log"))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			_, _ = fmt.Fprintf(output, "\n%s:\n%s", filepath.Base(path), data)
		}
	}
}

func parseRunID(t *testing.T, output string) string {
	t.Helper()
	match := regexp.MustCompile(`run_id:\s*([0-9A-HJKMNP-TV-Z]{26})`).FindStringSubmatch(output)
	if len(match) != 2 {
		t.Fatalf("run id missing from output:\n%s", output)
	}
	return match[1]
}
