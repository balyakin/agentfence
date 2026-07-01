package api

import (
	"time"

	"github.com/agentfence/agentfence/internal/domain"
)

type CreateRunRequest struct {
	RepoPath         string   `json:"repo_path" validate:"required"`
	Agent            string   `json:"agent" validate:"required"`
	Task             string   `json:"task"`
	TaskFile         string   `json:"task_file"`
	AgentArgs        []string `json:"agent_args"`
	TimeoutSeconds   int      `json:"timeout_seconds" validate:"min=1,max=86400"`
	Network          string   `json:"network" validate:"required,oneof=deny allow"`
	IncludeUntracked bool     `json:"include_untracked"`
	IncludeDirty     bool     `json:"include_dirty"`
	AllowSoftMode    bool     `json:"allow_soft_mode"`
}

type CancelRunRequest struct {
	RunID string `json:"run_id" validate:"required,ulid"`
}

type RunResponse struct {
	ID             string           `json:"id"`
	RepoPath       string           `json:"repo_path"`
	AgentName      string           `json:"agent_name"`
	Status         domain.RunStatus `json:"status"`
	TaskRedacted   string           `json:"task_redacted"`
	PreScanStatus  string           `json:"pre_scan_status"`
	PostScanStatus string           `json:"post_scan_status"`
	CreatedAt      time.Time        `json:"created_at"`
	FinishedAt     *time.Time       `json:"finished_at,omitempty"`
}

type CreateRunResponse struct {
	RunID  string           `json:"run_id"`
	Status domain.RunStatus `json:"status"`
}

type DiffResponse struct {
	RunID   string `json:"run_id"`
	Diff    string `json:"diff,omitempty"`
	Blocked bool   `json:"blocked"`
}

type ApplyRequest struct {
	CreateBranch   bool   `json:"create_branch"`
	BranchName     string `json:"branch_name"`
	AllowDirtyTree bool   `json:"allow_dirty_tree"`
	Reject         bool   `json:"reject"`
}

type ScanRequest struct {
	RepoPath         string `json:"repo_path" validate:"required"`
	IncludeUntracked bool   `json:"include_untracked"`
	IncludeDirty     bool   `json:"include_dirty"`
}

func runToResponse(run domain.Run) RunResponse {
	return RunResponse{
		ID: run.ID, RepoPath: run.RepoPath, AgentName: run.AgentName, Status: run.Status, TaskRedacted: run.TaskRedacted,
		PreScanStatus: run.PreScanStatus, PostScanStatus: run.PostScanStatus, CreatedAt: run.CreatedAt, FinishedAt: run.FinishedAt,
	}
}
