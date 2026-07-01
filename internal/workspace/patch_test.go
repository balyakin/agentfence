package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/agentfence/agentfence/internal/domain"
)

func TestGeneratePatchCreatesFile(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	shadow := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = shadow
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "a@example.invalid")
	run("config", "user.name", "A")
	if err := os.WriteFile(filepath.Join(shadow, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	run("add", "-A")
	run("commit", "-m", "base")
	patch := filepath.Join(t.TempDir(), "changes.patch")
	if err := NewManager(nil).GeneratePatch(context.Background(), domain.GeneratePatchRequest{ShadowPath: shadow, PatchPath: patch}); err != nil {
		t.Fatalf("patch: %v", err)
	}
	info, err := os.Stat(patch)
	if err != nil {
		t.Fatalf("stat patch: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("patch mode=%v", info.Mode().Perm())
	}
}
