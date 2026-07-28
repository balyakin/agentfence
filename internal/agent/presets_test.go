package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentfence/agentfence/internal/config"
	"github.com/agentfence/agentfence/internal/execx"
)

func TestPresetBatchEntrypointsExecute(t *testing.T) {
	tests := []struct {
		name     string
		firstArg string
	}{
		{name: "codex", firstArg: "exec"},
		{name: "claude", firstArg: "--print"},
		{name: "opencode", firstArg: "run"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			binDir := t.TempDir()
			executable := filepath.Join(binDir, testCase.name)
			script := "#!/bin/sh\n" +
				"test \"$1\" = \"" + testCase.firstArg + "\"\n" +
				"test \"$(cat)\" = \"task\"\n"
			if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
				t.Fatalf("write stub: %v", err)
			}
			t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			cfg := Presets()[testCase.name]
			invocation, err := NewGenericAdapter([]string{"PATH"}).Build(
				context.Background(),
				cfg,
				"task",
				nil,
			)
			if err != nil {
				t.Fatalf("build invocation: %v", err)
			}
			result, err := execx.NewProcessRunner(nil).Run(context.Background(), execx.ProcessRequest{
				Executable: invocation.Executable,
				Args:       invocation.Args,
				Env:        invocation.Env,
				Stdin:      strings.NewReader(invocation.Stdin),
			})
			if err != nil {
				t.Fatalf("run invocation: %v", err)
			}
			if result.ExitCode != 0 {
				t.Fatalf("exit code=%d", result.ExitCode)
			}
		})
	}
}

func TestRegistryBuildsDefaultAndOverrideInvocations(t *testing.T) {
	t.Setenv("AGENTFENCE_TEST_ENV", "allowed")
	cfg := config.AgentConfig{
		Default:      "shell",
		EnvAllowlist: []string{"", "AGENTFENCE_TEST_ENV", "AGENTFENCE_TEST_ENV", "MISSING_ENV"},
		Adapters: map[string]config.AgentAdapterConfig{
			"shell": {Command: "sh", Args: []string{"-c"}, TaskMode: "stdin"},
		},
	}
	registry := NewRegistry(cfg)
	invocation, err := registry.BuildInvocation(context.Background(), "", "task", []string{"extra"}, nil)
	if err != nil {
		t.Fatalf("build default invocation: %v", err)
	}
	if invocation.Stdin != "task" || len(invocation.Env) != 1 {
		t.Fatalf("unexpected default invocation: %#v", invocation)
	}
	override := config.AgentAdapterConfig{Command: "sh", TaskMode: "argv"}
	invocation, err = registry.WithConfig(cfg).BuildInvocation(
		context.Background(),
		"ignored",
		"task",
		nil,
		&override,
	)
	if err != nil {
		t.Fatalf("build override invocation: %v", err)
	}
	if len(invocation.Args) != 2 || invocation.Args[0] != "--message" || invocation.Args[1] != "task" {
		t.Fatalf("unexpected override invocation: %#v", invocation)
	}
}

func TestRegistryRejectsInvalidInvocation(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(config.AgentConfig{Adapters: map[string]config.AgentAdapterConfig{}})
	if _, err := registry.BuildInvocation(context.Background(), "missing", "task", nil, nil); err == nil {
		t.Fatal("missing adapter was accepted")
	}
	for _, adapter := range []config.AgentAdapterConfig{
		{},
		{Command: filepath.Join(t.TempDir(), "missing"), TaskMode: "stdin"},
		{Command: "sh", TaskMode: "unknown"},
	} {
		if _, err := registry.BuildInvocation(context.Background(), "", "task", nil, &adapter); err == nil {
			t.Fatalf("invalid adapter accepted: %#v", adapter)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := registry.BuildInvocation(
		ctx,
		"",
		"task",
		nil,
		&config.AgentAdapterConfig{Command: "sh", TaskMode: "stdin"},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled invocation error = %v", err)
	}
}
