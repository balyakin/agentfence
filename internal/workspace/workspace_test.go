package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/testutil"
)

func TestWorkspaceMetadataOutsideShadow(t *testing.T) {
	t.Parallel()
	git := &testutil.FakeGit{}
	manager := NewManager(git)
	runDir := t.TempDir()
	result, err := manager.Create(context.Background(), domain.CreateWorkspaceRequest{
		RepoRoot: runDir,
		RunDir:   filepath.Join(runDir, "run"),
		ExposurePlan: domain.ExposurePlan{Files: []domain.ExposureFile{
			{RelativePath: "main.go", Source: "head", Mode: 0o755, Size: 12},
		}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if filepath.Dir(result.MetadataPath) == result.ShadowPath {
		t.Fatalf("metadata written inside shadow")
	}
	info, err := os.Stat(filepath.Join(result.ShadowPath, "main.go"))
	if err != nil {
		t.Fatalf("stat copied file: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("executable bit not preserved: %v", info.Mode().Perm())
	}
}
