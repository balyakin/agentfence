package project

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentfence/agentfence/internal/domain"
)

func TestGitApplyPatchCanRollback(t *testing.T) {
	t.Parallel()
	repo := createApplyRepo(t)
	patch := createPatch(t, repo, "changed\n")
	git := NewGit()
	request := domain.ApplyPatchRequest{RepoPath: repo, PatchPath: patch}
	if err := git.ApplyPatch(context.Background(), request); err != nil {
		t.Fatalf("apply: %v", err)
	}
	assertFileContent(t, filepath.Join(repo, "file.txt"), "changed\n")
	if err := git.RollbackPatch(context.Background(), request); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	assertFileContent(t, filepath.Join(repo, "file.txt"), "base\n")
}

func TestGitApplyPatchCheckPreventsPartialChanges(t *testing.T) {
	t.Parallel()
	repo := createApplyRepo(t)
	patch := createPatch(t, repo, "changed\n")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("local\n"), 0o600); err != nil {
		t.Fatalf("write local change: %v", err)
	}
	err := NewGit().ApplyPatch(context.Background(), domain.ApplyPatchRequest{
		RepoPath: repo, PatchPath: patch, CreateBranch: true, BranchName: "agentfence/test", BaseRef: "main",
	})
	if !errors.Is(err, domain.ErrApplyConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	assertFileContent(t, filepath.Join(repo, "file.txt"), "local\n")
	branch := strings.TrimSpace(string(runApplyGit(t, repo, "branch", "--show-current")))
	if branch != "main" {
		t.Fatalf("branch changed before check: %s", branch)
	}
}

func TestGitInspectsRepositoryState(t *testing.T) {
	t.Parallel()
	repo := createApplyRepo(t)
	executablePath := filepath.Join(repo, "script.sh")
	if err := os.WriteFile(executablePath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}
	runApplyGit(t, repo, "add", "script.sh")
	runApplyGit(t, repo, "commit", "-m", "add executable")
	nestedDir := filepath.Join(repo, "nested")
	if err := os.Mkdir(nestedDir, 0o700); err != nil {
		t.Fatalf("create nested dir: %v", err)
	}
	git := NewGit()
	ctx := context.Background()

	root, err := git.FindRepoRoot(ctx, nestedDir)
	if err != nil {
		t.Fatalf("find repo root: %v", err)
	}
	expectedRoot, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatalf("resolve expected root: %v", err)
	}
	if root != expectedRoot {
		t.Fatalf("repo root = %q, want %q", root, expectedRoot)
	}
	head, err := git.HeadSHA(ctx, repo)
	if err != nil || head == "" {
		t.Fatalf("head = %q, error = %v", head, err)
	}
	baseRef, err := git.BaseRef(ctx, repo)
	if err != nil {
		t.Fatalf("base ref: %v", err)
	}
	if baseRef != "main" {
		t.Fatalf("base ref = %q, want main", baseRef)
	}
	tracked, err := git.TrackedFiles(ctx, repo)
	if err != nil {
		t.Fatalf("tracked files: %v", err)
	}
	if len(tracked) != 2 {
		t.Fatalf("tracked files = %#v", tracked)
	}
	blob, err := git.ReadBlob(ctx, repo, "script.sh")
	if err != nil {
		t.Fatalf("read blob: %v", err)
	}
	if !strings.Contains(string(blob), "exit 0") {
		t.Fatalf("unexpected executable blob: data=%q", blob)
	}

	if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("new\n"), 0o600); err != nil {
		t.Fatalf("write untracked: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("changed\n"), 0o600); err != nil {
		t.Fatalf("modify tracked: %v", err)
	}
	untracked, err := git.UntrackedFiles(ctx, repo)
	if err != nil {
		t.Fatalf("untracked files: %v", err)
	}
	if len(untracked) != 1 || untracked[0].RelativePath != "new.txt" {
		t.Fatalf("unexpected untracked files: %#v", untracked)
	}
	status, err := git.StatusPorcelain(ctx, repo)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(status) != 2 {
		t.Fatalf("unexpected status: %#v", status)
	}
	dirty, err := git.DirtyFiles(ctx, repo)
	if err != nil {
		t.Fatalf("dirty files: %v", err)
	}
	if len(dirty) != 1 || dirty[0].RelativePath != "file.txt" {
		t.Fatalf("unexpected dirty files: %#v", dirty)
	}
}

func TestGitRejectsNonRepositoryAndUnsafeBlob(t *testing.T) {
	t.Parallel()
	git := NewGit()
	if _, err := git.FindRepoRoot(context.Background(), t.TempDir()); !errors.Is(err, domain.ErrNotGitRepo) {
		t.Fatalf("non-repository error = %v", err)
	}
	if _, err := git.ReadBlob(context.Background(), t.TempDir(), "../outside"); !errors.Is(
		err,
		domain.ErrUnsafePath,
	) {
		t.Fatalf("unsafe blob error = %v", err)
	}
}

func TestGitApplyPatchIsIdempotent(t *testing.T) {
	t.Parallel()
	repo := createApplyRepo(t)
	patch := createPatch(t, repo, "changed\n")
	request := domain.ApplyPatchRequest{RepoPath: repo, PatchPath: patch}
	git := NewGit()
	if err := git.ApplyPatch(context.Background(), request); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if err := git.ApplyPatch(context.Background(), request); err != nil {
		t.Fatalf("second apply: %v", err)
	}
}

func createApplyRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runApplyGit(t, repo, "init", "-b", "main")
	runApplyGit(t, repo, "config", "user.email", "test@example.invalid")
	runApplyGit(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("base\n"), 0o600); err != nil {
		t.Fatalf("write base: %v", err)
	}
	runApplyGit(t, repo, "add", "file.txt")
	runApplyGit(t, repo, "commit", "-m", "base")
	return repo
}

func createPatch(t *testing.T, repo string, content string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte(content), 0o600); err != nil {
		t.Fatalf("write changed: %v", err)
	}
	patch := filepath.Join(t.TempDir(), "changes.patch")
	data := runApplyGit(t, repo, "diff", "--binary")
	if err := os.WriteFile(patch, data, 0o600); err != nil {
		t.Fatalf("write patch: %v", err)
	}
	runApplyGit(t, repo, "restore", "file.txt")
	return patch
}

func runApplyGit(t *testing.T, repo string, args ...string) []byte {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = repo
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return output
}

func assertFileContent(t *testing.T, path string, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != expected {
		t.Fatalf("content=%q want=%q", data, expected)
	}
}
