package workspace

import (
	"context"
	"errors"
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

func TestWorkspaceCopiesRegularWorktreeFile(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "dirty.txt"), []byte("dirty\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	result, err := NewManager(&testutil.FakeGit{}).Create(
		context.Background(),
		domain.CreateWorkspaceRequest{
			RepoRoot: repoRoot,
			RunDir:   filepath.Join(t.TempDir(), "run"),
			ExposurePlan: domain.ExposurePlan{Files: []domain.ExposureFile{
				{RelativePath: "dirty.txt", Source: "dirty", Mode: 0o600, Size: 6},
			}},
		},
	)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(result.ShadowPath, "dirty.txt"))
	if err != nil {
		t.Fatalf("read copied file: %v", err)
	}
	if string(data) != "dirty\n" {
		t.Fatalf("copied data=%q", data)
	}
}

func TestWorkspaceRejectsWorktreeFileThatGrewAfterPlanning(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "dirty.txt"), []byte("larger"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	_, err := NewManager(&testutil.FakeGit{}).Create(
		context.Background(),
		domain.CreateWorkspaceRequest{
			RepoRoot: repoRoot,
			RunDir:   filepath.Join(t.TempDir(), "run"),
			ExposurePlan: domain.ExposurePlan{Files: []domain.ExposureFile{
				{RelativePath: "dirty.txt", Source: "dirty", Mode: 0o600, Size: 1},
			}},
		},
	)
	if !errors.Is(err, domain.ErrUnsafePath) {
		t.Fatalf("error = %v, want unsafe path", err)
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

func TestValidatePostRunWorkspaceRejectsOversizedTree(t *testing.T) {
	t.Parallel()
	shadow := t.TempDir()
	path := filepath.Join(shadow, "oversized.bin")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("create oversized file: %v", err)
	}
	if err := file.Truncate(maxPostRunBytes + 1); err != nil {
		_ = file.Close()
		t.Fatalf("truncate oversized file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close oversized file: %v", err)
	}
	if err := NewManager(nil).ValidatePostRunWorkspace(context.Background(), shadow); err == nil {
		t.Fatalf("oversized worktree was accepted")
	}
}

func TestValidatePostRunWorkspaceRejectsSymlink(t *testing.T) {
	t.Parallel()
	shadow := t.TempDir()
	if err := os.Symlink("target", filepath.Join(shadow, "link")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	if err := NewManager(nil).ValidatePostRunWorkspace(context.Background(), shadow); !errors.Is(
		err,
		domain.ErrPostScanBlocked,
	) {
		t.Fatalf("error = %v, want post-scan blocked", err)
	}
}

func TestValidatePostRunWorkspaceAcceptsRegularTree(t *testing.T) {
	t.Parallel()
	shadow := t.TempDir()
	if err := os.Mkdir(filepath.Join(shadow, "nested"), 0o700); err != nil {
		t.Fatalf("create nested dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(shadow, "nested", "file.txt"), []byte("safe"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := NewManager(nil).ValidatePostRunWorkspace(context.Background(), shadow); err != nil {
		t.Fatalf("validate regular tree: %v", err)
	}
}

func TestValidatePostRunWorkspaceHonorsCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := NewManager(nil).ValidatePostRunWorkspace(ctx, t.TempDir()); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}

func TestWorkspaceRejectsNestedGitmodules(t *testing.T) {
	t.Parallel()
	manager := NewManager(&testutil.FakeGit{})
	_, err := manager.Create(context.Background(), domain.CreateWorkspaceRequest{
		RepoRoot: t.TempDir(),
		RunDir:   filepath.Join(t.TempDir(), "run"),
		ExposurePlan: domain.ExposurePlan{Files: []domain.ExposureFile{
			{RelativePath: "nested/.GITMODULES", Source: "head", Mode: 0o600},
		}},
	})
	if !errors.Is(err, domain.ErrUnsafePath) {
		t.Fatalf("error = %v, want unsafe path", err)
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
	if err := os.WriteFile(filepath.Join(result.ShadowPath, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatalf("write code: %v", err)
	}
	patchPath := filepath.Join(runDir, "changes.patch")
	err = manager.GeneratePatch(context.Background(), domain.GeneratePatchRequest{
		RunDir:            runDir,
		ShadowPath:        result.ShadowPath,
		TrustedGitDir:     result.TrustedGitDir,
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
	if !strings.Contains(string(patchData), "main.go") {
		t.Fatalf("new file missing from patch: %s", string(patchData))
	}
}
