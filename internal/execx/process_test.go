package execx

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestProcessRunnerTimeout(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := NewProcessRunner(nil).Run(ctx, ProcessRequest{Executable: "/bin/sleep", Args: []string{"5"}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestProcessRunnerCapturesExitAndOutput(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	result, err := NewProcessRunner(nil).Run(context.Background(), ProcessRequest{
		Executable: "/bin/sh",
		Args:       []string{"-c", "printf output; exit 7"},
		Stdout:     &output,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.ExitCode != 7 || output.String() != "output" {
		t.Fatalf("result=%#v output=%q", result, output.String())
	}
}

func TestProcessRunnerRejectsEmptyExecutable(t *testing.T) {
	t.Parallel()
	if _, err := NewProcessRunner(nil).Run(context.Background(), ProcessRequest{}); err == nil {
		t.Fatalf("empty executable was accepted")
	}
}

func TestProcessRunnerReportsStartFailure(t *testing.T) {
	t.Parallel()
	_, err := NewProcessRunner(nil).Run(context.Background(), ProcessRequest{
		Executable: filepath.Join(t.TempDir(), "missing"),
	})
	if err == nil {
		t.Fatal("missing executable was accepted")
	}
}

func TestProcessRunnerSuccessfulExit(t *testing.T) {
	t.Parallel()
	result, err := NewProcessRunner(nil).Run(context.Background(), ProcessRequest{
		Executable: "/bin/sh",
		Args:       []string{"-c", "exit 0"},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
}
