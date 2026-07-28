package workspace

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentfence/agentfence/internal/domain"
)

func TestGeneratePatchCreatesFile(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	runDir := t.TempDir()
	shadow := filepath.Join(runDir, "shadow")
	if err := os.MkdirAll(shadow, 0o700); err != nil {
		t.Fatalf("mkdir shadow: %v", err)
	}
	if err := os.WriteFile(filepath.Join(shadow, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(shadow, "delete.txt"), []byte("delete\n"), 0o644); err != nil {
		t.Fatalf("write deleted baseline: %v", err)
	}
	if err := os.WriteFile(filepath.Join(shadow, "binary.bin"), []byte{0, 1, 2}, 0o644); err != nil {
		t.Fatalf("write binary baseline: %v", err)
	}
	trustedGitDir := filepath.Join(runDir, "trusted.git")
	if err := initBaseline(context.Background(), shadow, trustedGitDir); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if err := os.WriteFile(filepath.Join(shadow, "a.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("modify: %v", err)
	}
	if err := os.WriteFile(filepath.Join(shadow, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatalf("write new: %v", err)
	}
	if err := os.Remove(filepath.Join(shadow, "delete.txt")); err != nil {
		t.Fatalf("delete file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(shadow, "binary.bin"), []byte{0, 3, 4}, 0o644); err != nil {
		t.Fatalf("modify binary: %v", err)
	}
	patch := filepath.Join(t.TempDir(), "changes.patch")
	if err := NewManager(nil).GeneratePatch(context.Background(), domain.GeneratePatchRequest{
		ShadowPath: shadow, TrustedGitDir: trustedGitDir, PatchPath: patch,
	}); err != nil {
		t.Fatalf("patch: %v", err)
	}
	info, err := os.Stat(patch)
	if err != nil {
		t.Fatalf("stat patch: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("patch mode=%v", info.Mode().Perm())
	}
	data, err := os.ReadFile(patch)
	if err != nil {
		t.Fatalf("read patch: %v", err)
	}
	patchText := string(data)
	if !strings.Contains(patchText, "new.txt") ||
		!strings.Contains(patchText, "changed") ||
		!strings.Contains(patchText, "deleted file mode") ||
		!strings.Contains(patchText, "GIT binary patch") {
		t.Fatalf("patch incomplete: %s", data)
	}
}

func TestWorkspaceRejectsAgentGitMetadata(t *testing.T) {
	t.Parallel()
	shadow := t.TempDir()
	gitDir := filepath.Join(shadow, ".git")
	if err := os.MkdirAll(gitDir, 0o700); err != nil {
		t.Fatalf("mkdir git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte("[diff]\n\texternal = /tmp/agent\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	err := NewManager(nil).ValidatePostRunWorkspace(context.Background(), shadow)
	if err == nil {
		t.Fatalf("agent-controlled git metadata was accepted")
	}
}

func TestGeneratePatchIgnoresAgentGitConfig(t *testing.T) {
	t.Parallel()
	runDir := t.TempDir()
	shadow := filepath.Join(runDir, "shadow")
	if err := os.MkdirAll(shadow, 0o700); err != nil {
		t.Fatalf("mkdir shadow: %v", err)
	}
	if err := os.WriteFile(filepath.Join(shadow, "file.txt"), []byte("base\n"), 0o600); err != nil {
		t.Fatalf("write baseline: %v", err)
	}
	trustedGitDir := filepath.Join(runDir, "trusted.git")
	if err := initBaseline(context.Background(), shadow, trustedGitDir); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	marker := filepath.Join(runDir, "host-marker")
	script := filepath.Join(shadow, "external-diff.sh")
	scriptData := []byte("#!/bin/sh\n: > \"" + marker + "\"\n")
	if err := os.WriteFile(script, scriptData, 0o700); err != nil {
		t.Fatalf("write external diff: %v", err)
	}
	gitDir := filepath.Join(shadow, ".git")
	if err := os.MkdirAll(gitDir, 0o700); err != nil {
		t.Fatalf("mkdir agent git: %v", err)
	}
	configData := []byte("[diff]\n\texternal = " + script + "\n")
	if err := os.WriteFile(filepath.Join(gitDir, "config"), configData, 0o600); err != nil {
		t.Fatalf("write agent config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(shadow, "file.txt"), []byte("changed\n"), 0o600); err != nil {
		t.Fatalf("modify file: %v", err)
	}
	patch := filepath.Join(runDir, "changes.patch")
	err := NewManager(nil).GeneratePatch(context.Background(), domain.GeneratePatchRequest{
		ShadowPath: shadow, TrustedGitDir: trustedGitDir, PatchPath: patch,
	})
	if err != nil {
		t.Fatalf("generate patch: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("agent external diff executed on host")
	}
	data, err := os.ReadFile(patch)
	if err != nil {
		t.Fatalf("read patch: %v", err)
	}
	if !strings.Contains(string(data), "changed") {
		t.Fatalf("trusted patch was altered: %s", data)
	}
}

func TestCleanIgnoredPatchPath(t *testing.T) {
	t.Parallel()
	path, err := cleanIgnoredPatchPath("generated/.env")
	if err != nil {
		t.Fatalf("clean path: %v", err)
	}
	if path != "generated/.env" {
		t.Fatalf("clean path = %q", path)
	}
	for _, unsafePath := range []string{"", ".", "../outside", "/absolute"} {
		if _, err := cleanIgnoredPatchPath(unsafePath); err == nil {
			t.Fatalf("unsafe path accepted: %q", unsafePath)
		}
	}
}

func TestLimitedWriterEnforcesLimitAndStoresWriteError(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	writer := &limitedWriter{dst: &output, remaining: 2}
	if _, err := writer.Write([]byte("abc")); !errors.Is(err, domain.ErrPostScanBlocked) {
		t.Fatalf("limit error = %v", err)
	}
	expectedErr := errors.New("write failed")
	writer = &limitedWriter{dst: errorWriter{err: expectedErr}, remaining: 10}
	if _, err := writer.Write([]byte("abc")); !errors.Is(err, expectedErr) {
		t.Fatalf("write error = %v", err)
	}
	if !errors.Is(writer.Error(), expectedErr) {
		t.Fatalf("stored error = %v", writer.Error())
	}
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write(data []byte) (int, error) {
	return 0, w.err
}
