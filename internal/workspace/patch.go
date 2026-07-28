package workspace

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentfence/agentfence/internal/domain"
)

const maxPatchBytes = 50 << 20

func (m *Manager) GeneratePatch(ctx context.Context, req domain.GeneratePatchRequest) error {
	pathspec := []string{"--", "."}
	if len(req.IgnoredPatchPaths) > 0 {
		for _, path := range req.IgnoredPatchPaths {
			clean, err := cleanIgnoredPatchPath(path)
			if err != nil {
				return err
			}
			pathspec = append(pathspec, ":(exclude)"+clean)
		}
	}
	addArgs := append([]string{"add", "-A"}, pathspec...)
	if err := runTrustedGit(ctx, req.TrustedGitDir, req.ShadowPath, nil, addArgs...); err != nil {
		return fmt.Errorf("stage final workspace: %w", err)
	}
	file, err := os.OpenFile(req.PatchPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create patch file: %w", err)
	}
	writer := &limitedWriter{dst: file, remaining: maxPatchBytes}
	diffArgs := []string{"diff", "--cached", "--binary", "--no-ext-diff", "--no-textconv", "HEAD"}
	diffArgs = append(diffArgs, pathspec...)
	runErr := runTrustedGit(ctx, req.TrustedGitDir, req.ShadowPath, writer, diffArgs...)
	if runErr != nil {
		_ = file.Close()
		return removeFileOnError(req.PatchPath, fmt.Errorf("generate git patch: %w", runErr))
	}
	if writer.Error() != nil {
		_ = file.Close()
		return removeFileOnError(req.PatchPath, fmt.Errorf("generate git patch: %w", writer.Error()))
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close patch file: %w", err)
	}
	if err := os.Chmod(req.PatchPath, 0o600); err != nil {
		return fmt.Errorf("chmod patch file: %w", err)
	}
	return nil
}

var _ io.Writer = (*limitedWriter)(nil)

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
