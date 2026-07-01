package workspace

import (
	"context"
	"fmt"
)

func initBaseline(ctx context.Context, shadowPath string) error {
	commands := [][]string{
		{"init"},
		{"config", "user.email", "agentfence@example.invalid"},
		{"config", "user.name", "AgentFence"},
		{"add", "-A"},
		{"commit", "--allow-empty", "-m", "agentfence baseline"},
	}
	for _, args := range commands {
		if _, err := runGit(ctx, shadowPath, args...); err != nil {
			return fmt.Errorf("initialize shadow git: %w", err)
		}
	}
	return nil
}
