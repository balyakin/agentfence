package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/errorsx"
)

func writeOutput(out io.Writer, jsonMode bool, value any) error {
	if jsonMode {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	}
	_, err := fmt.Fprintln(out, value)
	return err
}

func writePublicError(out io.Writer, err error) error {
	public, ok := errorsx.IsPublic(err)
	if !ok {
		public = errorsx.Wrap(errorsx.CodeInternal, "internal error", errorsx.ExitInternal, err)
	}
	encoder := json.NewEncoder(out)
	return encoder.Encode(public)
}

func writePublicErrorAndReturn(out io.Writer, err error) error {
	if writeErr := writePublicError(out, err); writeErr != nil {
		return writeErr
	}
	return err
}

func runToMap(run domain.Run) map[string]any {
	return map[string]any{
		"id":               run.ID,
		"repo_path":        run.RepoPath,
		"agent_name":       run.AgentName,
		"status":           run.Status,
		"task_redacted":    run.TaskRedacted,
		"pre_scan_status":  run.PreScanStatus,
		"post_scan_status": run.PostScanStatus,
	}
}
