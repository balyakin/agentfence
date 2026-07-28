package policy

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentfence/agentfence/internal/domain"
)

func TestSafeJoin(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "normal", path: "src/main.go"},
		{name: "parent", path: "../etc/passwd", wantErr: true},
		{name: "git upper", path: ".GIT/config", wantErr: true},
		{name: "absolute", path: "/etc/passwd", wantErr: true},
		{name: "nul", path: "a\x00b", wantErr: true},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := SafeJoin("/repo", testCase.path)
			if testCase.wantErr && err == nil {
				t.Fatalf("expected error")
			}
			if !testCase.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateSymlinkInsideRejectsChainedOutside(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	linkB := filepath.Join(root, "b")
	if err := os.Symlink(outsideFile, linkB); err != nil {
		t.Fatalf("symlink b: %v", err)
	}
	linkA := filepath.Join(root, "a")
	if err := os.Symlink("b", linkA); err != nil {
		t.Fatalf("symlink a: %v", err)
	}
	if err := ValidateSymlinkInside(root, linkA); !errors.Is(err, domain.ErrUnsafePath) {
		t.Fatalf("expected unsafe path for chained outside symlink, got %v", err)
	}
}

func TestValidateSymlinkInsideAcceptsRegularAndInternalLink(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("safe"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := ValidateSymlinkInside(root, target); err != nil {
		t.Fatalf("validate regular file: %v", err)
	}
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink("target.txt", link); err != nil {
		t.Fatalf("create link: %v", err)
	}
	if err := ValidateSymlinkInside(root, link); err != nil {
		t.Fatalf("validate internal link: %v", err)
	}
}

func TestOpenRegularNoSymlinks(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tasks"), 0o700); err != nil {
		t.Fatalf("mkdir tasks: %v", err)
	}
	path := filepath.Join(root, "tasks", "task.txt")
	if err := os.WriteFile(path, []byte("task"), 0o600); err != nil {
		t.Fatalf("write task: %v", err)
	}
	file, err := OpenRegularNoSymlinks(root, "tasks/task.txt")
	if err != nil {
		t.Fatalf("open regular: %v", err)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		_ = file.Close()
		t.Fatalf("read regular: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close regular: %v", err)
	}
	if string(data) != "task" {
		t.Fatalf("data=%q", data)
	}
}

func TestOpenRegularNoSymlinksRejectsFinalAndIntermediateLinks(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir()
	outsidePath := filepath.Join(outside, "task.txt")
	if err := os.WriteFile(outsidePath, []byte("task"), 0o600); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(root, "final.txt")); err != nil {
		t.Fatalf("symlink final: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "middle")); err != nil {
		t.Fatalf("symlink middle: %v", err)
	}
	for _, path := range []string{"final.txt", "middle/task.txt"} {
		if file, err := OpenRegularNoSymlinks(root, path); err == nil {
			_ = file.Close()
			t.Fatalf("symlink path accepted: %s", path)
		}
	}
}

func TestOpenRegularNoSymlinksRejectsDirectory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if file, err := OpenRegularNoSymlinks(root, "."); err == nil {
		_ = file.Close()
		t.Fatalf("directory was accepted")
	}
}

func FuzzSafeJoin(f *testing.F) {
	f.Add("src/main.go")
	f.Fuzz(func(t *testing.T, path string) {
		_, _ = SafeJoin("/repo", path)
	})
}
