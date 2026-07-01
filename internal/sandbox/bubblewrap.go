package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/errorsx"
	"github.com/agentfence/agentfence/internal/execx"
	"github.com/agentfence/agentfence/internal/ports"
)

type Bubblewrap struct {
	runner *execx.ProcessRunner
}

var _ ports.Sandbox = (*Bubblewrap)(nil)

func NewBubblewrap(runner *execx.ProcessRunner) *Bubblewrap {
	if runner == nil {
		runner = execx.NewProcessRunner(nil)
	}
	return &Bubblewrap{runner: runner}
}

func (b *Bubblewrap) Run(ctx context.Context, req domain.SandboxRunRequest) (domain.SandboxRunResult, error) {
	if runtime.GOOS != "linux" {
		return domain.SandboxRunResult{}, errorsx.Wrap(errorsx.CodeUnsupportedSandbox, "bubblewrap hard-mode is only supported on Linux", errorsx.ExitSandboxUnavailable, nil)
	}
	if _, err := exec.LookPath("bwrap"); err != nil {
		return domain.SandboxRunResult{}, errorsx.Wrap(errorsx.CodeBwrapNotFound, "bubblewrap not found", errorsx.ExitSandboxUnavailable, err)
	}
	argv, err := BuildBubblewrapArgv(req)
	if err != nil {
		return domain.SandboxRunResult{}, err
	}
	stdin := strings.NewReader(req.Invocation.Stdin)
	result, err := b.runner.Run(ctx, execx.ProcessRequest{
		Executable: "bwrap",
		Args:       argv,
		Stdin:      stdin,
		Stdout:     req.Stdout,
		Stderr:     req.Stderr,
	})
	return domain.SandboxRunResult{ExitCode: result.ExitCode, IsolationLevel: "bubblewrap", StartedAt: result.StartedAt, FinishedAt: result.FinishedAt}, err
}

func BuildBubblewrapArgv(req domain.SandboxRunRequest) ([]string, error) {
	if req.ShadowPath == "" || req.Invocation.Executable == "" {
		return nil, errorsx.Wrap(errorsx.CodeValidation, "sandbox request is incomplete", errorsx.ExitUsage, nil)
	}
	args := []string{
		"--die-with-parent",
		"--new-session",
		"--clearenv",
		"--unshare-user",
		"--unshare-ipc",
		"--unshare-pid",
		"--unshare-uts",
		"--unshare-cgroup",
		"--setenv", "PATH", "/usr/local/bin:/usr/bin:/bin",
		"--setenv", "HOME", "/agent-home",
		"--setenv", "AGENTFENCE_SANDBOX", "1",
		"--proc", "/proc",
		"--dev", "/dev",
	}
	if req.NetworkMode == "deny" || req.NetworkMode == "" {
		args = append(args, "--unshare-net")
	}
	tmpfsPaths := req.TmpfsPaths
	if len(tmpfsPaths) == 0 {
		tmpfsPaths = []string{"/tmp", "/agent-home"}
	}
	for _, path := range tmpfsPaths {
		args = append(args, "--tmpfs", path)
	}
	for _, path := range req.WritablePaths {
		if path == "" || path == "/workspace" || containsString(tmpfsPaths, path) {
			continue
		}
		args = append(args, "--tmpfs", path)
	}
	for _, path := range req.ReadonlySysPaths {
		if path == "" || path == os.Getenv("HOME") {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			args = append(args, "--ro-bind", path, path)
		}
	}
	for _, mount := range req.AuthMounts {
		if mount.HostPath == "" || mount.SandboxPath == "" {
			return nil, fmt.Errorf("invalid auth mount")
		}
		if mount.Readonly {
			args = append(args, "--ro-bind", mount.HostPath, mount.SandboxPath)
		} else {
			args = append(args, "--bind", mount.HostPath, mount.SandboxPath)
		}
	}
	args = append(args,
		"--bind", req.ShadowPath, "/workspace",
		"--chdir", "/workspace",
	)
	for _, env := range req.Invocation.Env {
		key, value, ok := strings.Cut(env, "=")
		if !ok || key == "" {
			continue
		}
		args = append(args, "--setenv", key, value)
	}
	args = append(args, req.Invocation.Executable)
	args = append(args, req.Invocation.Args...)
	return args, nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
