package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/agentfence/agentfence/internal/config"
	"github.com/agentfence/agentfence/internal/domain"
)

type GenericAdapter struct {
	envAllowlist []string
}

func NewGenericAdapter(envAllowlist []string) *GenericAdapter {
	return &GenericAdapter{envAllowlist: append([]string{}, envAllowlist...)}
}

func (a *GenericAdapter) Build(ctx context.Context, cfg config.AgentAdapterConfig, task string, agentArgs []string) (domain.Invocation, error) {
	if err := ctx.Err(); err != nil {
		return domain.Invocation{}, err
	}
	if cfg.Command == "" {
		return domain.Invocation{}, fmt.Errorf("agent command is empty")
	}
	executable, err := exec.LookPath(cfg.Command)
	if err != nil {
		return domain.Invocation{}, fmt.Errorf("agent command not found: %w", err)
	}
	args := append([]string{}, cfg.Args...)
	args = append(args, agentArgs...)
	stdin := ""
	switch cfg.TaskMode {
	case "stdin":
		stdin = task
	case "argv":
		args = append(args, "--message", task)
	default:
		return domain.Invocation{}, fmt.Errorf("unsupported task mode %q", cfg.TaskMode)
	}
	return domain.Invocation{
		Executable: executable,
		Args:       args,
		Stdin:      stdin,
		Env:        allowedEnv(a.envAllowlist),
	}, nil
}

func allowedEnv(keys []string) []string {
	result := make([]string, 0, len(keys))
	seen := map[string]struct{}{}
	for _, key := range keys {
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if value, ok := os.LookupEnv(key); ok {
			result = append(result, key+"="+value)
		}
	}
	return result
}
