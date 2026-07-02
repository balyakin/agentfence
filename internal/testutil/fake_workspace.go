package testutil

import (
	"context"
	"os"
	"path/filepath"

	"github.com/agentfence/agentfence/internal/domain"
)

type FakeWorkspace struct {
	PatchBytes        []byte
	SanitizedEnv      []string
	IgnoredPatchPaths []string
}

func (w *FakeWorkspace) Create(ctx context.Context, req domain.CreateWorkspaceRequest) (domain.WorkspaceResult, error) {
	shadow := filepath.Join(req.RunDir, "shadow")
	logs := filepath.Join(req.RunDir, "logs")
	scanner := filepath.Join(req.RunDir, "scanner")
	if err := os.MkdirAll(shadow, 0o700); err != nil {
		return domain.WorkspaceResult{}, err
	}
	if err := os.MkdirAll(logs, 0o700); err != nil {
		return domain.WorkspaceResult{}, err
	}
	if err := os.MkdirAll(scanner, 0o700); err != nil {
		return domain.WorkspaceResult{}, err
	}
	return domain.WorkspaceResult{
		RunDir:            req.RunDir,
		ShadowPath:        shadow,
		LogsDir:           logs,
		ScannerDir:        scanner,
		MetadataPath:      filepath.Join(req.RunDir, "shadow_metadata.json"),
		PatchPath:         filepath.Join(req.RunDir, "changes.patch"),
		SanitizedEnv:      append([]string{}, w.SanitizedEnv...),
		IgnoredPatchPaths: append([]string{}, w.IgnoredPatchPaths...),
	}, nil
}

func (w *FakeWorkspace) GeneratePatch(ctx context.Context, req domain.GeneratePatchRequest) error {
	return os.WriteFile(req.PatchPath, w.PatchBytes, 0o600)
}

func (w *FakeWorkspace) ValidatePostRunWorkspace(ctx context.Context, shadowPath string) error {
	return nil
}
