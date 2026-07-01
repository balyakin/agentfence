package testutil

import "context"

type FakeLockManager struct{}

func (FakeLockManager) Acquire(ctx context.Context, repoPath string) (func(), error) {
	return func() {}, nil
}
