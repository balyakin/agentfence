package config

import (
	"context"
	"strings"
	"testing"
)

func TestMinimalConfigKeepsDefaultExclude(t *testing.T) {
	t.Parallel()
	cfg, err := LoadBytes(context.Background(), []byte("version: 1\n"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !contains(cfg.Workspace.Exclude, ".env") {
		t.Fatalf("default .env exclude missing: %#v", cfg.Workspace.Exclude)
	}
}

func TestExcludeReplace(t *testing.T) {
	t.Parallel()
	cfg, err := LoadBytes(context.Background(), []byte("version: 1\nworkspace:\n  exclude_replace: true\n  exclude:\n    - build/**\n"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if contains(cfg.Workspace.Exclude, ".env") {
		t.Fatalf("exclude_replace kept default excludes: %#v", cfg.Workspace.Exclude)
	}
	if !contains(cfg.Workspace.Exclude, "build/**") {
		t.Fatalf("user exclude missing")
	}
}

func TestInvalidConfig(t *testing.T) {
	t.Parallel()
	tests := []string{
		"version: 1\nagent:\n  env_allowlist: [bad-key]\n",
		"version: 1\nsandbox:\n  network: proxy\n",
		"version: 1\nscan:\n  engines: [unknown]\n",
		"version: 1\napply_on_success: true\n",
	}
	for _, input := range tests {
		input := input
		t.Run(strings.Split(input, "\n")[1], func(t *testing.T) {
			t.Parallel()
			if _, err := LoadBytes(context.Background(), []byte(input)); err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestMergeDoesNotMutateBaseAdapters(t *testing.T) {
	t.Parallel()
	base := DefaultConfig()
	user := rawConfig{Agent: &rawAgentConfig{Adapters: map[string]rawAgentAdapterConfig{
		"codex": {Command: ptrString("custom-codex")},
	}}}
	merged := Merge(base, user)
	if merged.Agent.Adapters["codex"].Command != "custom-codex" {
		t.Fatalf("merged adapter command was not updated")
	}
	if base.Agent.Adapters["codex"].Command != "codex" {
		t.Fatalf("base adapter was mutated: %#v", base.Agent.Adapters["codex"])
	}
}

func FuzzConfigLoad(f *testing.F) {
	f.Add("version: 1\n")
	f.Fuzz(func(t *testing.T, input string) {
		_, _ = LoadBytes(context.Background(), []byte(input))
	})
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func ptrString(value string) *string {
	return &value
}
