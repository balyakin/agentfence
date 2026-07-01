package domain

import "errors"

var (
	ErrRunNotFound     = errors.New("run not found")
	ErrNotGitRepo      = errors.New("not a git repository")
	ErrRepoDirty       = errors.New("repository has uncommitted changes")
	ErrUnsafePath      = errors.New("unsafe path")
	ErrScanBlocked     = errors.New("scan blocked")
	ErrPostScanBlocked = errors.New("postflight scan blocked")
	ErrApplyConflict   = errors.New("apply conflict")
	ErrDependency      = errors.New("dependency missing")
	ErrValidation      = errors.New("validation failed")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrActiveRunExists = errors.New("active run exists")
)
