package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/agentfence/agentfence/internal/config"
	"github.com/agentfence/agentfence/internal/state"
)

func TestDoctorServiceCheck(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	result := NewDoctorService(state.Paths{StateDir: dir, CacheDir: dir, RunsDir: dir, LocksDir: dir, DBPath: filepath.Join(dir, "agentfence.db")}).Check(context.Background(), config.DefaultConfig())
	if len(result.Checks) == 0 {
		t.Fatalf("no checks")
	}
}
