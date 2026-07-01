package workspace

import (
	"context"
	"fmt"
	"os"

	"github.com/agentfence/agentfence/internal/domain"
)

func (m *Manager) GeneratePatch(ctx context.Context, req domain.GeneratePatchRequest) error {
	output, err := runGit(ctx, req.ShadowPath, "diff", "--binary", "HEAD")
	if err != nil {
		return fmt.Errorf("generate git patch: %w", err)
	}
	file, err := os.OpenFile(req.PatchPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create patch file: %w", err)
	}
	if _, err := file.Write(output); err != nil {
		closeErr := file.Close()
		if closeErr != nil {
			return fmt.Errorf("write patch file: %w; close patch file: %w", err, closeErr)
		}
		return fmt.Errorf("write patch file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close patch file: %w", err)
	}
	if err := os.Chmod(req.PatchPath, 0o600); err != nil {
		return fmt.Errorf("chmod patch file: %w", err)
	}
	return nil
}
