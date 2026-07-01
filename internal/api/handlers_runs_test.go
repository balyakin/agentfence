package api

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestTaskFromCreateRunRequestRejectsAbsoluteFile(t *testing.T) {
	t.Parallel()
	_, err := taskFromCreateRunRequest(CreateRunRequest{TaskFile: "/tmp/task.txt"}, t.TempDir())
	if err == nil {
		t.Fatalf("expected absolute task_file rejection")
	}
}

func TestTaskFromCreateRunRequestRejectsLargeFile(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	taskPath := filepath.Join(repoRoot, "task.txt")
	data := bytes.Repeat([]byte("a"), maxTaskFileBytes+1)
	if err := os.WriteFile(taskPath, data, 0o600); err != nil {
		t.Fatalf("write task file: %v", err)
	}
	_, err := taskFromCreateRunRequest(CreateRunRequest{TaskFile: "task.txt"}, repoRoot)
	if err == nil {
		t.Fatalf("expected large task_file rejection")
	}
}
