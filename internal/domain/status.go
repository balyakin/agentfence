package domain

type RunStatus string

const (
	RunStatusCreated     RunStatus = "created"
	RunStatusPreparing   RunStatus = "preparing"
	RunStatusScanning    RunStatus = "scanning"
	RunStatusBlocked     RunStatus = "blocked"
	RunStatusRunning     RunStatus = "running"
	RunStatusPostScan    RunStatus = "post_scanning"
	RunStatusBlockedPost RunStatus = "blocked_post"
	RunStatusSucceeded   RunStatus = "succeeded"
	RunStatusFailed      RunStatus = "failed"
	RunStatusTimedOut    RunStatus = "timed_out"
	RunStatusCancelled   RunStatus = "cancelled"
	RunStatusApplying    RunStatus = "applying"
	RunStatusApplied     RunStatus = "applied"
	RunStatusApplyFailed RunStatus = "apply_failed"
	RunStatusCleaned     RunStatus = "cleaned"
)

func (s RunStatus) IsTerminal() bool {
	switch s {
	case RunStatusBlocked, RunStatusBlockedPost, RunStatusSucceeded, RunStatusFailed,
		RunStatusTimedOut, RunStatusCancelled, RunStatusApplied, RunStatusApplyFailed, RunStatusCleaned:
		return true
	default:
		return false
	}
}

func (s RunStatus) IsActive() bool {
	switch s {
	case RunStatusCreated, RunStatusPreparing, RunStatusScanning, RunStatusRunning, RunStatusPostScan, RunStatusApplying:
		return true
	default:
		return false
	}
}
