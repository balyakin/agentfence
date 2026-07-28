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

func TestTaskFromCreateRunRequestRejectsSymlink(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	outsidePath := filepath.Join(t.TempDir(), "task.txt")
	if err := os.WriteFile(outsidePath, []byte("secret task"), 0o600); err != nil {
		t.Fatalf("write outside task: %v", err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(repoRoot, "task.txt")); err != nil {
		t.Fatalf("create task symlink: %v", err)
	}
	_, err := taskFromCreateRunRequest(CreateRunRequest{TaskFile: "task.txt"}, repoRoot)
	if err == nil {
		t.Fatalf("task symlink was accepted")
	}
}

func TestTaskFromCreateRunRequestRejectsIntermediateSymlink(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	outsideDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideDir, "task.txt"), []byte("secret task"), 0o600); err != nil {
		t.Fatalf("write outside task: %v", err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(repoRoot, "tasks")); err != nil {
		t.Fatalf("create directory symlink: %v", err)
	}
	_, err := taskFromCreateRunRequest(CreateRunRequest{TaskFile: "tasks/task.txt"}, repoRoot)
	if err == nil {
		t.Fatalf("intermediate task symlink was accepted")
	}
}
