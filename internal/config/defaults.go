package config

func DefaultConfig() Config {
	return Config{
		Version: 1,
		Workspace: WorkspaceConfig{
			MaxFileSizeBytes: 5 * 1024 * 1024,
			RequireCleanTree: true,
			IncludeUntracked: false,
			IncludeDirty:     false,
			Exclude:          MandatoryExcludes(),
			Include:          []string{"**"},
		},
		Scan: ScanConfig{
			Enabled:           true,
			Engines:           []string{"gitleaks"},
			FailOnFindings:    true,
			TimeoutSeconds:    120,
			ScanGitHistory:    false,
			SeverityBlocklist: []string{"critical", "high", "medium"},
		},
		Sandbox: SandboxConfig{
			Mode:          "bubblewrap",
			Network:       "deny",
			AllowSoftMode: false,
			WritablePaths: []string{"/workspace"},
			ReadonlySystemPaths: []string{
				"/usr", "/bin", "/lib", "/lib64", "/etc/ssl", "/etc/alternatives",
			},
			TmpfsPaths: []string{"/tmp", "/agent-home"},
		},
		Agent: AgentConfig{
			Default:      "codex",
			EnvAllowlist: []string{"PATH", "TERM"},
			SanitizedEnv: SanitizedEnvConfig{
				Enabled: true,
				ExampleFiles: []string{
					".env.example",
					".env.template",
					".env.sample",
				},
			},
			Adapters: map[string]AgentAdapterConfig{
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
			},
		},
		Apply: ApplyConfig{
			RequireCleanTree: true,
			CreateBranch:     true,
			BranchPrefix:     "agentfence/",
		},
		Daemon: DaemonConfig{
			Enabled:    false,
			MaxWorkers: 2,
		},
		Retention: RetentionConfig{
			MaxRunsPerRepo: 100,
			MaxAgeDays:     30,
			KeepFailed:     true,
		},
		Logging: LoggingConfig{
			Level: "info",
			JSON:  false,
		},
	}
}

func MandatoryExcludes() []string {
	return []string{
		".git",
		".git/**",
		".gitmodules",
		".env",
		".env.*",
		"**/*.pem",
		"**/*.key",
		"**/*_rsa",
		"**/*_dsa",
		"**/*_ecdsa",
		"**/*_ed25519",
		"**/id_rsa",
		"**/id_dsa",
		"**/id_ecdsa",
		"**/id_ed25519",
		"**/.aws/**",
		"**/.gcp/**",
		"**/.azure/**",
		"**/*.sqlite",
		"**/*.sqlite3",
		"**/*.db",
		"**/*.dump",
		"**/*.sql",
		"node_modules/**",
		"vendor/**",
	}
}
