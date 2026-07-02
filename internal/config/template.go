package config

import (
	"bytes"
	"text/template"
)

const DefaultTemplate = `# AgentFence v1 configuration.
# Soft mode is not a sandbox. Use it only with --allow-soft-mode.
version: 1

workspace:
  state_dir: ""
  cache_dir: ""
  max_file_size_bytes: 5242880
  require_clean_tree: true
  include_untracked: false
  include_dirty: false
  exclude_replace: false
  include:
    - "**"
  exclude:
    - ".git"
    - ".git/**"
    - ".gitmodules"
    - ".env"
    - ".env.*"
    - "**/*.pem"
    - "**/*.key"
    - "**/*_rsa"
    - "**/*_dsa"
    - "**/*_ecdsa"
    - "**/*_ed25519"
    - "**/id_rsa"
    - "**/id_dsa"
    - "**/id_ecdsa"
    - "**/id_ed25519"
    - "**/.aws/**"
    - "**/.gcp/**"
    - "**/.azure/**"
    - "**/*.sqlite"
    - "**/*.sqlite3"
    - "**/*.db"
    - "**/*.dump"
    - "**/*.sql"
    - "node_modules/**"
    - "vendor/**"

scan:
  enabled: true
  engines:
    - gitleaks
  fail_on_findings: true
  timeout_seconds: 120
  scan_git_history: false
  severity_blocklist:
    - critical
    - high
    - medium

sandbox:
  mode: bubblewrap
  network: deny
  allow_soft_mode: false
  writable_paths:
    - /workspace
  readonly_system_paths:
    - /usr
    - /bin
    - /lib
    - /lib64
    - /etc/ssl
    - /etc/alternatives
  tmpfs_paths:
    - /tmp
    - /agent-home

agent:
  default: codex
  interactive: false
  auth_mounts: []
  env_allowlist:
    - PATH
    - TERM
  sanitized_env:
    enabled: true
    example_files:
      - ".env.example"
      - ".env.template"
      - ".env.sample"
    extra_keys: []
  adapters:
    codex:
      command: codex
      args: ["--no-interactive"]
      task_mode: stdin
    claude:
      command: claude
      args: ["--non-interactive"]
      task_mode: stdin
    aider:
      command: aider
      args: []
      task_mode: argv
    opencode:
      command: opencode
      args: []
      task_mode: stdin

apply:
  require_clean_tree: true
  create_branch: true
  branch_prefix: "agentfence/"

daemon:
  enabled: false
  socket_path: ""
  listen_address: ""
  token_file: ""
  max_workers: 2

retention:
  max_runs_per_repo: 100
  max_age_days: 30
  keep_failed: true

logging:
  level: info
  json: false
`

func RenderTemplate() ([]byte, error) {
	tmpl, err := template.New("agentfence-config").Parse(DefaultTemplate)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, nil); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
