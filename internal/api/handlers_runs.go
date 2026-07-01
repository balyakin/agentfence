package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/errorsx"
	"github.com/agentfence/agentfence/internal/policy"
	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
)

const maxTaskFileBytes = 1 << 20

func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	var req CreateRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, errorsx.Wrap(errorsx.CodeValidation, "invalid json body", errorsx.ExitUsage, err))
		return
	}
	if err := s.validator.Struct(req); err != nil {
		writeError(w, errorsx.Wrap(errorsx.CodeValidation, "request validation failed", errorsx.ExitUsage, err))
		return
	}
	repoRoot, err := s.git.FindRepoRoot(r.Context(), req.RepoPath)
	if err != nil {
		writeError(w, errorsx.Wrap(errorsx.CodeNotGitRepo, "not a git repository", errorsx.ExitUsage, err))
		return
	}
	task, err := taskFromCreateRunRequest(req, repoRoot)
	if err != nil {
		writeError(w, err)
		return
	}
	req.Task = task
	now := time.Now().UTC()
	runID, err := newRunID(now)
	if err != nil {
		writeError(w, errorsx.Wrap(errorsx.CodeInternal, "generate run id failed", errorsx.ExitInternal, err))
		return
	}
	runDir := s.paths.RunDir(runID)
	run := domain.Run{
		ID: runID, RepoPath: repoRoot, RunDir: runDir, ShadowPath: filepath.Join(runDir, "shadow"),
		MetadataPath: filepath.Join(runDir, "shadow_metadata.json"), PatchPath: filepath.Join(runDir, "changes.patch"),
		AgentName: req.Agent, TaskRedacted: s.redactor.RedactString(req.Task), Status: domain.RunStatusCreated,
		PreScanStatus: "pending", PostScanStatus: "pending", NetworkMode: req.Network, TimeoutSeconds: req.TimeoutSeconds,
		CreatedAt: now, UpdatedAt: now,
	}
	queueCtx := context.WithoutCancel(r.Context())
	if err := s.store.CreateRun(queueCtx, run); err != nil {
		writeError(w, err)
		return
	}
	if err := s.queue.Enqueue(queueCtx, RunJob{RunID: runID, Req: requestToRun(runID, req)}); err != nil {
		_ = s.store.SetRunFinished(context.WithoutCancel(r.Context()), domain.RunFinishResult{
			RunID:        runID,
			Status:       domain.RunStatusFailed,
			ErrorCode:    errorsx.CodeInternal,
			ErrorMessage: "failed to enqueue run",
			FinishedAt:   time.Now().UTC(),
		})
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, CreateRunResponse{RunID: runID, Status: domain.RunStatusCreated})
}

func taskFromCreateRunRequest(req CreateRunRequest, repoRoot string) (string, error) {
	hasTask := strings.TrimSpace(req.Task) != ""
	hasTaskFile := strings.TrimSpace(req.TaskFile) != ""
	if hasTask == hasTaskFile {
		return "", errorsx.Wrap(errorsx.CodeValidation, "exactly one of task or task_file is required", errorsx.ExitUsage, nil)
	}
	if hasTask {
		return strings.TrimSpace(req.Task), nil
	}
	if filepath.IsAbs(req.TaskFile) {
		return "", errorsx.Wrap(errorsx.CodeValidation, "task_file must be relative to repo", errorsx.ExitUsage, nil)
	}
	taskPath, err := policy.SafeJoin(repoRoot, req.TaskFile)
	if err != nil {
		return "", errorsx.Wrap(errorsx.CodeValidation, "invalid task_file path", errorsx.ExitUsage, err)
	}
	file, err := os.Open(taskPath)
	if err != nil {
		return "", errorsx.Wrap(errorsx.CodeValidation, "read task file failed", errorsx.ExitUsage, err)
	}
	defer func() {
		_ = file.Close()
	}()
	data, err := io.ReadAll(io.LimitReader(file, maxTaskFileBytes+1))
	if err != nil {
		return "", errorsx.Wrap(errorsx.CodeValidation, "read task file failed", errorsx.ExitUsage, err)
	}
	if len(data) > maxTaskFileBytes {
		return "", errorsx.Wrap(errorsx.CodeValidation, "task file too large", errorsx.ExitUsage, nil)
	}
	task := strings.TrimSpace(string(data))
	if task == "" {
		return "", errorsx.Wrap(errorsx.CodeValidation, "task is required", errorsx.ExitUsage, nil)
	}
	return task, nil
}

func newRunID(now time.Time) (string, error) {
	entropy := ulid.Monotonic(rand.Reader, 0)
	id, err := ulid.New(ulid.Timestamp(now), entropy)
	if err != nil {
		return "", fmt.Errorf("generate run id: %w", err)
	}
	return id.String(), nil
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		writeError(w, mapRunError(err))
		return
	}
	writeJSON(w, http.StatusOK, runToResponse(run))
}

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	repoPath := r.URL.Query().Get("repo_path")
	runs, err := s.store.ListRuns(r.Context(), domain.RunFilter{RepoPath: repoPath, Limit: 100})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) handleCancelRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	if _, err := ulid.Parse(runID); err != nil {
		writeError(w, errorsx.Wrap(errorsx.CodeValidation, "invalid run id", errorsx.ExitUsage, err))
		return
	}
	if !s.queue.Cancel(runID) {
		writeError(w, errorsx.Wrap(errorsx.CodeNotFound, "run is not active", errorsx.ExitNotFound, nil))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (s *Server) handleDiff(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		writeError(w, mapRunError(err))
		return
	}
	blocked := run.PostScanStatus == "blocked" || run.Status == domain.RunStatusBlockedPost
	data, err := os.ReadFile(run.PatchPath)
	if err != nil {
		writeError(w, errorsx.Wrap(errorsx.CodeNotFound, "patch file not found", errorsx.ExitNotFound, err))
		return
	}
	diff := s.redactor.RedactString(string(data))
	if blocked {
		diff = ""
	}
	writeJSON(w, http.StatusOK, DiffResponse{RunID: run.ID, Diff: diff, Blocked: blocked})
}

func mapRunError(err error) error {
	if errors.Is(err, domain.ErrRunNotFound) {
		return errorsx.Wrap(errorsx.CodeRunNotFound, "run not found", errorsx.ExitNotFound, err)
	}
	return err
}
