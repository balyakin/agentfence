CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS runs (
    id TEXT PRIMARY KEY,
    repo_path TEXT NOT NULL,
    repo_head TEXT NOT NULL,
    base_ref TEXT NOT NULL,
    run_dir TEXT NOT NULL,
    shadow_path TEXT NOT NULL,
    metadata_path TEXT NOT NULL,
    patch_path TEXT NOT NULL,
    agent_name TEXT NOT NULL,
    task_redacted TEXT NOT NULL,
    status TEXT NOT NULL,
    pre_scan_status TEXT NOT NULL DEFAULT 'pending',
    post_scan_status TEXT NOT NULL DEFAULT 'pending',
    network_mode TEXT NOT NULL,
    isolation_level TEXT NOT NULL,
    timeout_seconds INTEGER NOT NULL,
    exit_code INTEGER,
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    started_at TEXT,
    finished_at TEXT,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_runs_repo_created ON runs(repo_path, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);

CREATE TABLE IF NOT EXISTS run_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id TEXT NOT NULL,
    ts TEXT NOT NULL,
    level TEXT NOT NULL,
    event_type TEXT NOT NULL,
    message TEXT NOT NULL,
    data_json TEXT NOT NULL DEFAULT '{}',
    FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_run_events_run_ts ON run_events(run_id, ts);

CREATE TABLE IF NOT EXISTS scan_findings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id TEXT NOT NULL,
    phase TEXT NOT NULL,
    engine TEXT NOT NULL,
    file_path TEXT NOT NULL,
    line INTEGER NOT NULL DEFAULT 0,
    column_number INTEGER NOT NULL DEFAULT 0,
    rule_id TEXT NOT NULL,
    severity TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    secret_sha256 TEXT NOT NULL,
    redacted_secret TEXT NOT NULL,
    description TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_scan_findings_run ON scan_findings(run_id);
CREATE INDEX IF NOT EXISTS idx_scan_findings_phase ON scan_findings(run_id, phase);
CREATE INDEX IF NOT EXISTS idx_scan_findings_fingerprint ON scan_findings(fingerprint);

CREATE TABLE IF NOT EXISTS config_snapshots (
    run_id TEXT PRIMARY KEY,
    config_json TEXT NOT NULL,
    effective_policy_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS apply_attempts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id TEXT NOT NULL,
    repo_path TEXT NOT NULL,
    patch_path TEXT NOT NULL,
    strategy TEXT NOT NULL,
    branch_name TEXT NOT NULL DEFAULT '',
    reject_dir TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    finished_at TEXT,
    FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_apply_attempts_run ON apply_attempts(run_id);
