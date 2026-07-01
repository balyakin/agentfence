package ports

import "context"

type LockManager interface {
	Acquire(ctx context.Context, repoPath string) (release func(), err error)
}
