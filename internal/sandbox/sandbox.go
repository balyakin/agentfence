package sandbox

import (
	"fmt"

	"github.com/agentfence/agentfence/internal/config"
	"github.com/agentfence/agentfence/internal/errorsx"
	"github.com/agentfence/agentfence/internal/execx"
	"github.com/agentfence/agentfence/internal/ports"
)

func New(cfg config.SandboxConfig, runner *execx.ProcessRunner) (ports.Sandbox, error) {
	switch cfg.Mode {
	case "bubblewrap":
		return NewBubblewrap(runner), nil
	case "soft":
		return NewSoft(runner), nil
	default:
		return nil, errorsx.Wrap(errorsx.CodeUnsupportedSandbox, fmt.Sprintf("unsupported sandbox mode %q", cfg.Mode), errorsx.ExitUsage, nil)
	}
}
