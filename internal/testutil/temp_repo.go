package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TempRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o700); err != nil {
		t.Fatalf("create fake git dir: %v", err)
	}
	return dir
}
