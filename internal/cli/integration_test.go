package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIWorkflow(t *testing.T) {
	repo := t.TempDir()
	runCLIGit(t, repo, "init", "-b", "main")
	runCLIGit(t, repo, "config", "user.email", "test@example.invalid")
	runCLIGit(t, repo, "config", "user.name", "Test")
	configData := []byte(
		"version: 1\n" +
			"sandbox:\n  mode: soft\n  allow_soft_mode: true\n" +
			"apply:\n  create_branch: false\n",
	)
	if err := os.WriteFile(filepath.Join(repo, ".agentfence.yml"), configData, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0o600); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	runCLIGit(t, repo, "add", ".")
	runCLIGit(t, repo, "commit", "-m", "base")

	binDir := t.TempDir()
	gitleaksPath := filepath.Join(binDir, "gitleaks")
	gitleaksScript := `#!/bin/sh
if [ "${1:-}" = "version" ]; then
	exit 0
fi
report=""
while [ "$#" -gt 0 ]; do
	if [ "$1" = "--report-path" ]; then
		shift
		report="$1"
	fi
	shift
done
test -n "$report"
printf '[]\n' > "$report"
`
	if err := os.WriteFile(gitleaksPath, []byte(gitleaksScript), 0o700); err != nil {
		t.Fatalf("write gitleaks stub: %v", err)
	}
	agentPath := filepath.Join(binDir, "agent")
	if err := os.WriteFile(agentPath, []byte("#!/bin/sh\nprintf 'change\\n' > cli-change.txt\n"), 0o700); err != nil {
		t.Fatalf("write agent stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(previousDir)
	})

	if output, err := executeCLI("doctor"); err != nil {
		t.Fatalf("doctor: %v: %s", err, output)
	}
	if output, err := executeCLI("scan"); err != nil {
		t.Fatalf("scan: %v: %s", err, output)
	}
	runOutput, err := executeCLI(
		"run",
		"generic",
		"--command",
		agentPath,
		"--task",
		"make a change",
		"--timeout",
		"30",
	)
	if err != nil {
		t.Fatalf("run: %v: %s", err, runOutput)
	}
	if !strings.Contains(runOutput, "run_id:") {
		t.Fatalf("run output=%s", runOutput)
	}
	if output, err := executeCLI("status"); err != nil {
		t.Fatalf("status: %v: %s", err, output)
	}
	diffOutput, err := executeCLI("diff", "latest")
	if err != nil {
		t.Fatalf("diff: %v: %s", err, diffOutput)
	}
	if !strings.Contains(diffOutput, "cli-change.txt") {
		t.Fatalf("diff output=%s", diffOutput)
	}
	if output, err := executeCLI("diff", "latest", "--stat"); err != nil {
		t.Fatalf("diff stat: %v: %s", err, output)
	}
	if output, err := executeCLI("apply", "latest"); err != nil {
		t.Fatalf("apply: %v: %s", err, output)
	}
	if _, err := os.Stat(filepath.Join(repo, "cli-change.txt")); err != nil {
		t.Fatalf("applied file missing: %v", err)
	}
	if output, err := executeCLI("clean", "--dry-run"); err != nil {
		t.Fatalf("clean: %v: %s", err, output)
	}
}

func TestCLIInit(t *testing.T) {
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(previousDir)
	})
	if output, err := executeCLI("init"); err != nil {
		t.Fatalf("init: %v: %s", err, output)
	}
	if _, err := executeCLI("init"); err == nil {
		t.Fatalf("second init succeeded")
	}
}

func executeCLI(args ...string) (string, error) {
	command := NewRootCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs(args)
	err := command.ExecuteContext(context.Background())
	return output.String(), err
}

func runCLIGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = repo
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
