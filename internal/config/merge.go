package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type rawConfig struct {
	Version   *int                `yaml:"version"`
	Workspace *rawWorkspaceConfig `yaml:"workspace"`
	Scan      *rawScanConfig      `yaml:"scan"`
	Sandbox   *rawSandboxConfig   `yaml:"sandbox"`
	Agent     *rawAgentConfig     `yaml:"agent"`
	Apply     *rawApplyConfig     `yaml:"apply"`
	Daemon    *rawDaemonConfig    `yaml:"daemon"`
	Retention *rawRetentionConfig `yaml:"retention"`
	Logging   *rawLoggingConfig   `yaml:"logging"`
}

type rawWorkspaceConfig struct {
	StateDir         *string  `yaml:"state_dir"`
	CacheDir         *string  `yaml:"cache_dir"`
	MaxFileSizeBytes *int64   `yaml:"max_file_size_bytes"`
	RequireCleanTree *bool    `yaml:"require_clean_tree"`
	IncludeUntracked *bool    `yaml:"include_untracked"`
	IncludeDirty     *bool    `yaml:"include_dirty"`
	Exclude          []string `yaml:"exclude"`
	ExcludeReplace   *bool    `yaml:"exclude_replace"`
	Include          []string `yaml:"include"`
}

type rawScanConfig struct {
	Enabled           *bool    `yaml:"enabled"`
	Engines           []string `yaml:"engines"`
	FailOnFindings    *bool    `yaml:"fail_on_findings"`
	TimeoutSeconds    *int     `yaml:"timeout_seconds"`
	ScanGitHistory    *bool    `yaml:"scan_git_history"`
	SeverityBlocklist []string `yaml:"severity_blocklist"`
}

type rawSandboxConfig struct {
	Mode                *string  `yaml:"mode"`
	Network             *string  `yaml:"network"`
	AllowSoftMode       *bool    `yaml:"allow_soft_mode"`
	WritablePaths       []string `yaml:"writable_paths"`
	ReadonlySystemPaths []string `yaml:"readonly_system_paths"`
	TmpfsPaths          []string `yaml:"tmpfs_paths"`
}

type rawAgentConfig struct {
	Default      *string                          `yaml:"default"`
	Interactive  *bool                            `yaml:"interactive"`
	AuthMounts   []AuthMount                      `yaml:"auth_mounts"`
	EnvAllowlist []string                         `yaml:"env_allowlist"`
	SanitizedEnv *rawSanitizedEnvConfig           `yaml:"sanitized_env"`
	Adapters     map[string]rawAgentAdapterConfig `yaml:"adapters"`
}

type rawSanitizedEnvConfig struct {
	Enabled      *bool    `yaml:"enabled"`
	ExampleFiles []string `yaml:"example_files"`
	ExtraKeys    []string `yaml:"extra_keys"`
}

type rawAgentAdapterConfig struct {
	Command  *string  `yaml:"command"`
	Args     []string `yaml:"args"`
	TaskMode *string  `yaml:"task_mode"`
}

type rawApplyConfig struct {
	RequireCleanTree *bool   `yaml:"require_clean_tree"`
	CreateBranch     *bool   `yaml:"create_branch"`
	BranchPrefix     *string `yaml:"branch_prefix"`
}

type rawDaemonConfig struct {
	Enabled       *bool   `yaml:"enabled"`
	SocketPath    *string `yaml:"socket_path"`
	ListenAddress *string `yaml:"listen_address"`
	TokenFile     *string `yaml:"token_file"`
	MaxWorkers    *int    `yaml:"max_workers"`
}

type rawRetentionConfig struct {
	MaxRunsPerRepo *int  `yaml:"max_runs_per_repo"`
	MaxAgeDays     *int  `yaml:"max_age_days"`
	KeepFailed     *bool `yaml:"keep_failed"`
}

type rawLoggingConfig struct {
	Level *string `yaml:"level"`
	JSON  *bool   `yaml:"json"`
}

func Load(ctx context.Context, path string) (Config, error) {
	if err := ctx.Err(); err != nil {
		return Config{}, err
	}
	if path == "" {
		cfg := DefaultConfig()
		if err := Validate(cfg); err != nil {
			return Config{}, err
		}
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg := DefaultConfig()
			if err := Validate(cfg); err != nil {
				return Config{}, err
			}
			return cfg, nil
		}
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	return LoadBytes(ctx, data)
}

func LoadForRepo(ctx context.Context, repoRoot string, explicitPath string) (Config, error) {
	path := explicitPath
	if path == "" {
		path = filepath.Join(repoRoot, ".agentfence.yml")
	}
	return Load(ctx, path)
}

func LoadBytes(ctx context.Context, data []byte) (Config, error) {
	if err := ctx.Err(); err != nil {
		return Config{}, err
	}
	user, err := decodeRaw(data)
	if err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	cfg := Merge(DefaultConfig(), user)
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func decodeRaw(data []byte) (rawConfig, error) {
	var user rawConfig
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&user); err != nil {
		return rawConfig{}, err
	}
	return user, nil
}

func Merge(base Config, user rawConfig) Config {
	out := cloneConfig(base)
	if user.Version != nil {
		out.Version = *user.Version
	}
	if user.Workspace != nil {
		mergeWorkspace(&out.Workspace, *user.Workspace)
	}
	if user.Scan != nil {
		mergeScan(&out.Scan, *user.Scan)
	}
	if user.Sandbox != nil {
		mergeSandbox(&out.Sandbox, *user.Sandbox)
	}
	if user.Agent != nil {
		mergeAgent(&out.Agent, *user.Agent)
	}
	if user.Apply != nil {
		mergeApply(&out.Apply, *user.Apply)
	}
	if user.Daemon != nil {
		mergeDaemon(&out.Daemon, *user.Daemon)
	}
	if user.Retention != nil {
		mergeRetention(&out.Retention, *user.Retention)
	}
	if user.Logging != nil {
		mergeLogging(&out.Logging, *user.Logging)
	}
	return out
}

func cloneConfig(in Config) Config {
	out := in
	out.Workspace.Exclude = append([]string{}, in.Workspace.Exclude...)
	out.Workspace.Include = append([]string{}, in.Workspace.Include...)
	out.Scan.Engines = append([]string{}, in.Scan.Engines...)
	out.Scan.SeverityBlocklist = append([]string{}, in.Scan.SeverityBlocklist...)
	out.Sandbox.WritablePaths = append([]string{}, in.Sandbox.WritablePaths...)
	out.Sandbox.ReadonlySystemPaths = append([]string{}, in.Sandbox.ReadonlySystemPaths...)
	out.Sandbox.TmpfsPaths = append([]string{}, in.Sandbox.TmpfsPaths...)
	out.Agent.AuthMounts = append([]AuthMount{}, in.Agent.AuthMounts...)
	out.Agent.EnvAllowlist = append([]string{}, in.Agent.EnvAllowlist...)
	out.Agent.SanitizedEnv.ExampleFiles = append([]string{}, in.Agent.SanitizedEnv.ExampleFiles...)
	out.Agent.SanitizedEnv.ExtraKeys = append([]string{}, in.Agent.SanitizedEnv.ExtraKeys...)
	if in.Agent.Adapters != nil {
		out.Agent.Adapters = make(map[string]AgentAdapterConfig, len(in.Agent.Adapters))
		for name, adapter := range in.Agent.Adapters {
			adapter.Args = append([]string{}, adapter.Args...)
			out.Agent.Adapters[name] = adapter
		}
	}
	return out
}

func mergeWorkspace(dst *WorkspaceConfig, user rawWorkspaceConfig) {
	if user.StateDir != nil {
		dst.StateDir = *user.StateDir
	}
	if user.CacheDir != nil {
		dst.CacheDir = *user.CacheDir
	}
	if user.MaxFileSizeBytes != nil {
		dst.MaxFileSizeBytes = *user.MaxFileSizeBytes
	}
	if user.RequireCleanTree != nil {
		dst.RequireCleanTree = *user.RequireCleanTree
	}
	if user.IncludeUntracked != nil {
		dst.IncludeUntracked = *user.IncludeUntracked
	}
	if user.IncludeDirty != nil {
		dst.IncludeDirty = *user.IncludeDirty
	}
	if user.ExcludeReplace != nil {
		dst.ExcludeReplace = *user.ExcludeReplace
	}
	if user.Exclude != nil {
		if dst.ExcludeReplace {
			dst.Exclude = append([]string{}, user.Exclude...)
		} else {
			dst.Exclude = appendUnique(dst.Exclude, user.Exclude...)
		}
	}
	if len(user.Include) > 0 {
		dst.Include = append([]string{}, user.Include...)
	}
}

func mergeScan(dst *ScanConfig, user rawScanConfig) {
	if user.Enabled != nil {
		dst.Enabled = *user.Enabled
	}
	if user.Engines != nil {
		dst.Engines = append([]string{}, user.Engines...)
	}
	if user.FailOnFindings != nil {
		dst.FailOnFindings = *user.FailOnFindings
	}
	if user.TimeoutSeconds != nil {
		dst.TimeoutSeconds = *user.TimeoutSeconds
	}
	if user.ScanGitHistory != nil {
		dst.ScanGitHistory = *user.ScanGitHistory
	}
	if user.SeverityBlocklist != nil {
		dst.SeverityBlocklist = append([]string{}, user.SeverityBlocklist...)
	}
}

func mergeSandbox(dst *SandboxConfig, user rawSandboxConfig) {
	if user.Mode != nil {
		dst.Mode = *user.Mode
	}
	if user.Network != nil {
		dst.Network = *user.Network
	}
	if user.AllowSoftMode != nil {
		dst.AllowSoftMode = *user.AllowSoftMode
	}
	if user.WritablePaths != nil {
		dst.WritablePaths = append([]string{}, user.WritablePaths...)
	}
	if user.ReadonlySystemPaths != nil {
		dst.ReadonlySystemPaths = append([]string{}, user.ReadonlySystemPaths...)
	}
	if user.TmpfsPaths != nil {
		dst.TmpfsPaths = append([]string{}, user.TmpfsPaths...)
	}
}

func mergeAgent(dst *AgentConfig, user rawAgentConfig) {
	if user.Default != nil {
		dst.Default = *user.Default
	}
	if user.Interactive != nil {
		dst.Interactive = *user.Interactive
	}
	if user.AuthMounts != nil {
		dst.AuthMounts = append([]AuthMount{}, user.AuthMounts...)
	}
	if user.EnvAllowlist != nil {
		dst.EnvAllowlist = appendUnique(dst.EnvAllowlist, user.EnvAllowlist...)
	}
	if user.SanitizedEnv != nil {
		mergeSanitizedEnv(&dst.SanitizedEnv, *user.SanitizedEnv)
	}
	if user.Adapters != nil {
		if dst.Adapters == nil {
			dst.Adapters = map[string]AgentAdapterConfig{}
		}
		for name, raw := range user.Adapters {
			adapter := dst.Adapters[name]
			if raw.Command != nil {
				adapter.Command = *raw.Command
			}
			if raw.Args != nil {
				adapter.Args = append([]string{}, raw.Args...)
			}
			if raw.TaskMode != nil {
				adapter.TaskMode = *raw.TaskMode
			}
			dst.Adapters[name] = adapter
		}
	}
}

func mergeSanitizedEnv(dst *SanitizedEnvConfig, user rawSanitizedEnvConfig) {
	if user.Enabled != nil {
		dst.Enabled = *user.Enabled
	}
	if user.ExampleFiles != nil {
		dst.ExampleFiles = append([]string{}, user.ExampleFiles...)
	}
	if user.ExtraKeys != nil {
		dst.ExtraKeys = appendUnique(dst.ExtraKeys, user.ExtraKeys...)
	}
}

func mergeApply(dst *ApplyConfig, user rawApplyConfig) {
	if user.RequireCleanTree != nil {
		dst.RequireCleanTree = *user.RequireCleanTree
	}
	if user.CreateBranch != nil {
		dst.CreateBranch = *user.CreateBranch
	}
	if user.BranchPrefix != nil {
		dst.BranchPrefix = *user.BranchPrefix
	}
}

func mergeDaemon(dst *DaemonConfig, user rawDaemonConfig) {
	if user.Enabled != nil {
		dst.Enabled = *user.Enabled
	}
	if user.SocketPath != nil {
		dst.SocketPath = *user.SocketPath
	}
	if user.ListenAddress != nil {
		dst.ListenAddress = *user.ListenAddress
	}
	if user.TokenFile != nil {
		dst.TokenFile = *user.TokenFile
	}
	if user.MaxWorkers != nil {
		dst.MaxWorkers = *user.MaxWorkers
	}
}

func mergeRetention(dst *RetentionConfig, user rawRetentionConfig) {
	if user.MaxRunsPerRepo != nil {
		dst.MaxRunsPerRepo = *user.MaxRunsPerRepo
	}
	if user.MaxAgeDays != nil {
		dst.MaxAgeDays = *user.MaxAgeDays
	}
	if user.KeepFailed != nil {
		dst.KeepFailed = *user.KeepFailed
	}
}

func mergeLogging(dst *LoggingConfig, user rawLoggingConfig) {
	if user.Level != nil {
		dst.Level = *user.Level
	}
	if user.JSON != nil {
		dst.JSON = *user.JSON
	}
}

func appendUnique(dst []string, src ...string) []string {
	seen := make(map[string]struct{}, len(dst)+len(src))
	out := make([]string, 0, len(dst)+len(src))
	for _, item := range dst {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	for _, item := range src {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
