package config

type Config struct {
	Version   int             `yaml:"version" json:"version" validate:"required,eq=1"`
	Workspace WorkspaceConfig `yaml:"workspace" json:"workspace" validate:"required"`
	Scan      ScanConfig      `yaml:"scan" json:"scan" validate:"required"`
	Sandbox   SandboxConfig   `yaml:"sandbox" json:"sandbox" validate:"required"`
	Agent     AgentConfig     `yaml:"agent" json:"agent" validate:"required"`
	Apply     ApplyConfig     `yaml:"apply" json:"apply" validate:"required"`
	Daemon    DaemonConfig    `yaml:"daemon" json:"daemon" validate:"required"`
	Retention RetentionConfig `yaml:"retention" json:"retention" validate:"required"`
	Logging   LoggingConfig   `yaml:"logging" json:"logging" validate:"required"`
}

type WorkspaceConfig struct {
	StateDir         string   `yaml:"state_dir" json:"state_dir"`
	CacheDir         string   `yaml:"cache_dir" json:"cache_dir"`
	MaxFileSizeBytes int64    `yaml:"max_file_size_bytes" json:"max_file_size_bytes" validate:"min=1,max=104857600"`
	RequireCleanTree bool     `yaml:"require_clean_tree" json:"require_clean_tree"`
	IncludeUntracked bool     `yaml:"include_untracked" json:"include_untracked"`
	IncludeDirty     bool     `yaml:"include_dirty" json:"include_dirty"`
	Exclude          []string `yaml:"exclude" json:"exclude" validate:"required,min=1,dive,min=1"`
	ExcludeReplace   bool     `yaml:"exclude_replace" json:"exclude_replace"`
	Include          []string `yaml:"include" json:"include" validate:"required,min=1,dive,min=1"`
}

type ScanConfig struct {
	Enabled           bool     `yaml:"enabled" json:"enabled"`
	Engines           []string `yaml:"engines" json:"engines" validate:"required,min=1,dive,oneof=gitleaks trufflehog"`
	FailOnFindings    bool     `yaml:"fail_on_findings" json:"fail_on_findings"`
	TimeoutSeconds    int      `yaml:"timeout_seconds" json:"timeout_seconds" validate:"min=1,max=1800"`
	ScanGitHistory    bool     `yaml:"scan_git_history" json:"scan_git_history"`
	SeverityBlocklist []string `yaml:"severity_blocklist" json:"severity_blocklist" validate:"dive,oneof=critical high medium low info"`
}

type SandboxConfig struct {
	Mode                string   `yaml:"mode" json:"mode" validate:"required,oneof=bubblewrap soft"`
	Network             string   `yaml:"network" json:"network" validate:"required,oneof=deny allow"`
	AllowSoftMode       bool     `yaml:"allow_soft_mode" json:"allow_soft_mode"`
	WritablePaths       []string `yaml:"writable_paths" json:"writable_paths" validate:"required,min=1,dive,startswith=/"`
	ReadonlySystemPaths []string `yaml:"readonly_system_paths" json:"readonly_system_paths" validate:"dive,startswith=/"`
	TmpfsPaths          []string `yaml:"tmpfs_paths" json:"tmpfs_paths" validate:"dive,startswith=/"`
}

type AgentConfig struct {
	Default      string                        `yaml:"default" json:"default" validate:"required"`
	Interactive  bool                          `yaml:"interactive" json:"interactive"`
	AuthMounts   []AuthMount                   `yaml:"auth_mounts" json:"auth_mounts" validate:"dive"`
	EnvAllowlist []string                      `yaml:"env_allowlist" json:"env_allowlist" validate:"dive,envkey"`
	SanitizedEnv SanitizedEnvConfig            `yaml:"sanitized_env" json:"sanitized_env" validate:"required"`
	Adapters     map[string]AgentAdapterConfig `yaml:"adapters" json:"adapters" validate:"required,min=1,dive"`
}

type SanitizedEnvConfig struct {
	Enabled      bool     `yaml:"enabled" json:"enabled"`
	ExampleFiles []string `yaml:"example_files" json:"example_files" validate:"dive,min=1"`
	ExtraKeys    []string `yaml:"extra_keys" json:"extra_keys" validate:"dive,envkey"`
}

type AuthMount struct {
	HostPath    string `yaml:"host_path" json:"host_path" validate:"required"`
	SandboxPath string `yaml:"sandbox_path" json:"sandbox_path" validate:"required,startswith=/"`
	Readonly    bool   `yaml:"readonly" json:"readonly"`
	Writable    bool   `yaml:"writable" json:"writable"`
}

type AgentAdapterConfig struct {
	Command  string   `yaml:"command" json:"command" validate:"required"`
	Args     []string `yaml:"args" json:"args"`
	TaskMode string   `yaml:"task_mode" json:"task_mode" validate:"required,oneof=stdin argv"`
}

type ApplyConfig struct {
	RequireCleanTree bool   `yaml:"require_clean_tree" json:"require_clean_tree"`
	CreateBranch     bool   `yaml:"create_branch" json:"create_branch"`
	BranchPrefix     string `yaml:"branch_prefix" json:"branch_prefix" validate:"required"`
}

type DaemonConfig struct {
	Enabled       bool   `yaml:"enabled" json:"enabled"`
	SocketPath    string `yaml:"socket_path" json:"socket_path"`
	ListenAddress string `yaml:"listen_address" json:"listen_address"`
	TokenFile     string `yaml:"token_file" json:"token_file"`
	MaxWorkers    int    `yaml:"max_workers" json:"max_workers" validate:"min=1,max=8"`
}

type RetentionConfig struct {
	MaxRunsPerRepo int  `yaml:"max_runs_per_repo" json:"max_runs_per_repo" validate:"min=1,max=1000"`
	MaxAgeDays     int  `yaml:"max_age_days" json:"max_age_days" validate:"min=1,max=365"`
	KeepFailed     bool `yaml:"keep_failed" json:"keep_failed"`
}

type LoggingConfig struct {
	Level string `yaml:"level" json:"level" validate:"required,oneof=debug info warn error"`
	JSON  bool   `yaml:"json" json:"json"`
}
