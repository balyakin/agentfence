package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentfence/agentfence/internal/domain"
)

func (m *Manager) GeneratePatch(ctx context.Context, req domain.GeneratePatchRequest) error {
	args := []string{"diff", "--binary", "HEAD"}
	if len(req.IgnoredPatchPaths) > 0 {
		args = append(args, "--", ".")
		for _, path := range req.IgnoredPatchPaths {
			clean, err := cleanIgnoredPatchPath(path)
			if err != nil {
				return err
			}
			args = append(args, ":(exclude)"+clean)
		}
	}
	output, err := runGit(ctx, req.ShadowPath, args...)
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

func cleanIgnoredPatchPath(path string) (string, error) {
	if path == "" || filepath.IsAbs(path) || containsParentSegment(path) {
		return "", fmt.Errorf("unsafe ignored patch path %q", path)
	}
	clean := filepath.Clean(path)
	slashPath := filepath.ToSlash(clean)
	if slashPath == "." {
		return "", fmt.Errorf("unsafe ignored patch path %q", path)
	}
	if strings.HasPrefix(slashPath, "/") {
		return "", fmt.Errorf("unsafe ignored patch path %q", path)
	}
	return slashPath, nil
}
