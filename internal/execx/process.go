package execx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"syscall"
	"time"
)

type ProcessRequest struct {
	Executable string
	Args       []string
	Dir        string
	Env        []string
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
}

type ProcessResult struct {
	ExitCode   int
	StartedAt  time.Time
	FinishedAt time.Time
}

type ProcessRunner struct {
	logger      *slog.Logger
	killGrace   time.Duration
	commandHook func(*exec.Cmd)
}

const hardKillWait = 5 * time.Second

func NewProcessRunner(logger *slog.Logger) *ProcessRunner {
	return &ProcessRunner{logger: logger, killGrace: 5 * time.Second}
}

func (r *ProcessRunner) Run(ctx context.Context, req ProcessRequest) (ProcessResult, error) {
	if req.Executable == "" {
		return ProcessResult{}, errors.New("empty executable")
	}
	started := time.Now().UTC()
	cmd := exec.Command(req.Executable, req.Args...)
	cmd.Dir = req.Dir
	cmd.Env = req.Env
	cmd.Stdin = req.Stdin
	cmd.Stdout = req.Stdout
	cmd.Stderr = req.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if r.commandHook != nil {
		r.commandHook(cmd)
	}
	if err := cmd.Start(); err != nil {
		return ProcessResult{}, fmt.Errorf("start process: %w", err)
	}
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Kill()
		select {
		case <-waitCh:
			return ProcessResult{}, fmt.Errorf("get process group: %w", err)
		case <-time.After(hardKillWait):
			return ProcessResult{}, fmt.Errorf("get process group: %w; process did not stop", err)
		}
	}

	select {
	case err := <-waitCh:
		return ProcessResult{ExitCode: exitCode(err), StartedAt: started, FinishedAt: time.Now().UTC()}, nil
	case <-ctx.Done():
		if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil && r.logger != nil {
			r.logger.ErrorContext(ctx, "send SIGTERM failed", slog.Int("pgid", pgid), slog.Any("error", err))
		}
		select {
		case err := <-waitCh:
			return ProcessResult{ExitCode: exitCode(err), StartedAt: started, FinishedAt: time.Now().UTC()}, ctx.Err()
		case <-time.After(r.killGrace):
			if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil && r.logger != nil {
				r.logger.ErrorContext(ctx, "send SIGKILL failed", slog.Int("pgid", pgid), slog.Any("error", err))
			}
			select {
			case err := <-waitCh:
				return ProcessResult{
					ExitCode: exitCode(err), StartedAt: started, FinishedAt: time.Now().UTC(),
				}, ctx.Err()
			case <-time.After(hardKillWait):
				return ProcessResult{
					ExitCode: -1, StartedAt: started, FinishedAt: time.Now().UTC(),
				}, ctx.Err()
			}
		}
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}
