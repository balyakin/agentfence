package sandbox

import (
	"context"
	"strings"

	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/errorsx"
	"github.com/agentfence/agentfence/internal/execx"
	"github.com/agentfence/agentfence/internal/ports"
)

type Soft struct {
	runner *execx.ProcessRunner
}

var _ ports.Sandbox = (*Soft)(nil)

func NewSoft(runner *execx.ProcessRunner) *Soft {
	if runner == nil {
		runner = execx.NewProcessRunner(nil)
	}
	return &Soft{runner: runner}
}

func (s *Soft) Run(ctx context.Context, req domain.SandboxRunRequest) (domain.SandboxRunResult, error) {
	if !req.AllowSoftMode {
		return domain.SandboxRunResult{}, errorsx.Wrap(errorsx.CodeSandboxUnavailable, "soft mode requires explicit allow_soft_mode", errorsx.ExitSandboxUnavailable, nil)
	}
	result, err := s.runner.Run(ctx, execx.ProcessRequest{
		Executable: req.Invocation.Executable,
		Args:       req.Invocation.Args,
		Dir:        req.ShadowPath,
		Env:        req.Invocation.Env,
		Stdin:      strings.NewReader(req.Invocation.Stdin),
		Stdout:     req.Stdout,
		Stderr:     req.Stderr,
	})
	return domain.SandboxRunResult{ExitCode: result.ExitCode, IsolationLevel: "soft", StartedAt: result.StartedAt, FinishedAt: result.FinishedAt}, err
}
