package workspace

import (
	"context"
	"fmt"
	"os"
)

func initBaseline(ctx context.Context, shadowPath string, trustedGitDir string) error {
	if _, err := runGit(ctx, shadowPath, "init", "--bare", trustedGitDir); err != nil {
		return fmt.Errorf("initialize trusted git directory: %w", err)
	}
	if err := os.Chmod(trustedGitDir, 0o700); err != nil {
		return fmt.Errorf("chmod trusted git directory: %w", err)
	}
	commands := [][]string{
		{"config", "user.email", "agentfence@example.invalid"},
		{"config", "user.name", "AgentFence"},
		{"config", "core.bare", "false"},
		{"config", "core.worktree", shadowPath},
		{"add", "-A"},
		{"commit", "--allow-empty", "-m", "agentfence baseline"},
	}
	for _, args := range commands {
		if err := runTrustedGit(ctx, trustedGitDir, shadowPath, nil, args...); err != nil {
			return fmt.Errorf("initialize shadow git: %w", err)
		}
	}
	return nil
}
