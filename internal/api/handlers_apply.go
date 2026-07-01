package api

import (
	"encoding/json"
	"net/http"

	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/errorsx"
	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
)

func (s *Server) handleApply(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	if _, err := ulid.Parse(runID); err != nil {
		writeError(w, errorsx.Wrap(errorsx.CodeValidation, "invalid run id", errorsx.ExitUsage, err))
		return
	}
	var req ApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, errorsx.Wrap(errorsx.CodeValidation, "invalid json body", errorsx.ExitUsage, err))
		return
	}
	run, err := s.applyService.Apply(r.Context(), domain.ApplyRunRequest{
		RunID: runID, CreateBranch: req.CreateBranch, BranchName: req.BranchName, AllowDirtyTree: req.AllowDirtyTree, Reject: req.Reject,
	}, "", "")
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, runToResponse(run))
}
