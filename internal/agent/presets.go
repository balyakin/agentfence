package agent

import "github.com/agentfence/agentfence/internal/config"

func Presets() map[string]config.AgentAdapterConfig {
	return map[string]config.AgentAdapterConfig{
		"codex": {
			Command:  "codex",
			Args:     []string{"--no-interactive"},
			TaskMode: "stdin",
		},
		"claude": {
			Command:  "claude",
			Args:     []string{"--non-interactive"},
			TaskMode: "stdin",
		},
		"aider": {
			Command:  "aider",
			Args:     []string{},
			TaskMode: "argv",
		},
		"opencode": {
			Command:  "opencode",
			Args:     []string{},
			TaskMode: "stdin",
		},
	}
}
