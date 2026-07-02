package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentfence/agentfence/internal/config"
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

func TestWorkspaceCreatesIgnoredSanitizedEnv(t *testing.T) {
	t.Parallel()
	git := &testutil.FakeGit{}
	manager := NewManager(git)
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, ".env.example"), []byte("DATABASE_URL=\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	runDir := filepath.Join(t.TempDir(), "run")
	result, err := manager.Create(context.Background(), domain.CreateWorkspaceRequest{
		RepoRoot: repoRoot,
		RunDir:   runDir,
		SanitizedEnv: config.SanitizedEnvConfig{
			Enabled:      true,
			ExampleFiles: []string{".env.example"},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	envPath := filepath.Join(result.ShadowPath, ".env")
	info, err := os.Stat(envPath)
	if err != nil {
		t.Fatalf("stat generated env: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%v", info.Mode().Perm())
	}
	if len(result.SanitizedEnv) != 1 {
		t.Fatalf("env=%v", result.SanitizedEnv)
	}
	if len(result.IgnoredPatchPaths) != 1 || result.IgnoredPatchPaths[0] != ".env" {
		t.Fatalf("ignored paths=%v", result.IgnoredPatchPaths)
	}
}

func TestGeneratePatchExcludesSanitizedEnv(t *testing.T) {
	t.Parallel()
	manager := NewManager(&testutil.FakeGit{})
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, ".env.example"), []byte("DATABASE_URL=\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	runDir := filepath.Join(t.TempDir(), "run")
	result, err := manager.Create(context.Background(), domain.CreateWorkspaceRequest{
		RepoRoot: repoRoot,
		RunDir:   runDir,
		SanitizedEnv: config.SanitizedEnvConfig{
			Enabled:      true,
			ExampleFiles: []string{".env.example"},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := os.WriteFile(filepath.Join(result.ShadowPath, ".env"), []byte("DATABASE_URL=changed\n"), 0o600); err != nil {
		t.Fatalf("modify env: %v", err)
	}
	if _, err := runGit(context.Background(), result.ShadowPath, "add", "-f", ".env"); err != nil {
		t.Fatalf("stage env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(result.ShadowPath, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatalf("write code: %v", err)
	}
	patchPath := filepath.Join(runDir, "changes.patch")
	err = manager.GeneratePatch(context.Background(), domain.GeneratePatchRequest{
		RunDir:            runDir,
		ShadowPath:        result.ShadowPath,
		PatchPath:         patchPath,
		IgnoredPatchPaths: result.IgnoredPatchPaths,
	})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	patchData, err := os.ReadFile(patchPath)
	if err != nil {
		t.Fatalf("read patch: %v", err)
	}
	if strings.Contains(string(patchData), "DATABASE_URL") {
		t.Fatalf("sanitized env leaked into patch: %s", string(patchData))
	}
}
