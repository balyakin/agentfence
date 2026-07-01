package domain

import (
	"io"
	"os"
	"time"

	"github.com/agentfence/agentfence/internal/config"
)

type Run struct {
	ID             string
	RepoPath       string
	RepoHead       string
	BaseRef        string
	RunDir         string
	ShadowPath     string
	MetadataPath   string
	PatchPath      string
	AgentName      string
	TaskRedacted   string
	Status         RunStatus
	PreScanStatus  string
	PostScanStatus string
	NetworkMode    string
	IsolationLevel string
	TimeoutSeconds int
	ExitCode       *int
	ErrorCode      string
	ErrorMessage   string
	CreatedAt      time.Time
	StartedAt      *time.Time
	FinishedAt     *time.Time
	UpdatedAt      time.Time
}

type RunFilter struct {
	RepoPath string
	Status   RunStatus
	Limit    int
}

type RunSummary struct {
	ID           string
	RepoPath     string
	AgentName    string
	Status       RunStatus
	TaskRedacted string
	CreatedAt    time.Time
	FinishedAt   *time.Time
}

type RunEvent struct {
	RunID     string
	Ts        time.Time
	Level     string
	EventType string
	Message   string
	DataJSON  string
}

type RunFinishResult struct {
	RunID          string
	Status         RunStatus
	ExitCode       *int
	IsolationLevel string
	ErrorCode      string
	ErrorMessage   string
	FinishedAt     time.Time
}

type ApplyAttempt struct {
	ID           int64
	RunID        string
	RepoPath     string
	PatchPath    string
	Strategy     string
	BranchName   string
	RejectDir    string
	Status       string
	ErrorCode    string
	ErrorMessage string
	CreatedAt    time.Time
	FinishedAt   *time.Time
}

type ApplyPatchRequest struct {
	RepoPath     string
	PatchPath    string
	CreateBranch bool
	BranchName   string
	Reject       bool
	RejectDir    string
}

type ApplyRunRequest struct {
	RunID          string `json:"run_id" validate:"required"`
	CreateBranch   bool   `json:"create_branch"`
	BranchName     string `json:"branch_name"`
	AllowDirtyTree bool   `json:"allow_dirty_tree"`
	Reject         bool   `json:"reject"`
}

type ExposureFile struct {
	RelativePath string
	Source       string
	Mode         os.FileMode
	Size         int64
}

type SkippedFile struct {
	RelativePath string `json:"relative_path"`
	Reason       string `json:"reason"`
	Size         int64  `json:"size,omitempty"`
}

type ExposurePlanRequest struct {
	RepoPath         string
	RepoHead         string
	BaseRef          string
	IncludeUntracked bool
	IncludeDirty     bool
	Config           config.Config
}

type ExposurePlan struct {
	Files        []ExposureFile `json:"files"`
	TotalSize    int64          `json:"total_size"`
	SkippedFiles []SkippedFile  `json:"skipped_files"`
}

type CreateWorkspaceRequest struct {
	RunID        string
	RepoRoot     string
	RunDir       string
	ExposurePlan ExposurePlan
	RepoHead     string
	BaseRef      string
}

type WorkspaceResult struct {
	RunDir       string
	ShadowPath   string
	ScannerDir   string
	LogsDir      string
	MetadataPath string
	PatchPath    string
}

type GeneratePatchRequest struct {
	RunDir     string
	ShadowPath string
	PatchPath  string
}

type SandboxRunRequest struct {
	Invocation       Invocation
	ShadowPath       string
	LogsDir          string
	NetworkMode      string
	TimeoutSeconds   int
	WritablePaths    []string
	AuthMounts       []config.AuthMount
	EnvAllowlist     []string
	Stdout           io.Writer
	Stderr           io.Writer
	IsolationHint    string
	AllowSoftMode    bool
	ReadonlySysPaths []string
	TmpfsPaths       []string
}

type SandboxRunResult struct {
	ExitCode       int
	IsolationLevel string
	StartedAt      time.Time
	FinishedAt     time.Time
}

type Invocation struct {
	Executable string
	Args       []string
	Stdin      string
	Env        []string
}

type StatusEntry struct {
	Path     string
	OrigPath string
	Index    byte
	Worktree byte
}

type GitFile struct {
	RelativePath string
	Mode         os.FileMode
	Size         int64
	IsSubmodule  bool
}

type WorktreeFile struct {
	RelativePath string
	Mode         os.FileMode
	Size         int64
}

type RunRequest struct {
	RunID            string
	RepoPath         string
	Agent            string
	Task             string
	AgentArgs        []string
	TimeoutSeconds   int
	Network          string
	IncludeUntracked bool
	IncludeDirty     bool
	AllowSoftMode    bool
	AdapterOverride  *config.AgentAdapterConfig
	ConfigPath       string
}

type RunResult struct {
	RunID     string    `json:"run_id"`
	Status    RunStatus `json:"status"`
	PatchPath string    `json:"patch_path,omitempty"`
}
