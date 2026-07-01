package api

import (
	"encoding/json"
	"net/http"

	"github.com/agentfence/agentfence/internal/app"
	"github.com/agentfence/agentfence/internal/errorsx"
)

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	var req ScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, errorsx.Wrap(errorsx.CodeValidation, "invalid json body", errorsx.ExitUsage, err))
		return
	}
	if err := s.validator.Struct(req); err != nil {
		writeError(w, errorsx.Wrap(errorsx.CodeValidation, "request validation failed", errorsx.ExitUsage, err))
		return
	}
	result, err := s.scanService.Scan(r.Context(), app.ScanOnlyRequest{RepoPath: req.RepoPath, IncludeUntracked: req.IncludeUntracked, IncludeDirty: req.IncludeDirty})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
