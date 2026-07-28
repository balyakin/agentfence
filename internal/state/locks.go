package state

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/ports"
	"github.com/gofrs/flock"
)

type FileLockManager struct {
	paths  Paths
	logger *slog.Logger
}

var _ ports.LockManager = (*FileLockManager)(nil)

func NewFileLockManager(paths Paths, logger *slog.Logger) *FileLockManager {
	return &FileLockManager{paths: paths, logger: logger}
}

func (m *FileLockManager) Acquire(ctx context.Context, repoPath string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	lock := flock.New(m.paths.LockPath(repoPath))
	ok, err := lock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("acquire repo lock: %w", err)
	}
	if !ok {
		return nil, domain.ErrActiveRunExists
	}
	return func() {
		if err := lock.Unlock(); err != nil && m.logger != nil {
			m.logger.ErrorContext(ctx, "release repo lock failed", slog.Any("error", err))
		}
	}, nil
}
