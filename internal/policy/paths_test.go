package policy

import (
	"errors"
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

func FuzzSafeJoin(f *testing.F) {
	f.Add("src/main.go")
	f.Fuzz(func(t *testing.T, path string) {
		_, _ = SafeJoin("/repo", path)
	})
}
