package execx

import (
	"context"
	"errors"
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
