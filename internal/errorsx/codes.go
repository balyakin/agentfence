package errorsx

const (
	CodeInternal                   = "AF_INTERNAL"
	CodeConfigInvalid              = "AF_CONFIG_INVALID"
	CodeConfigExists               = "AF_CONFIG_EXISTS"
	CodeValidation                 = "AF_VALIDATION"
	CodeNotFound                   = "AF_NOT_FOUND"
	CodeRunNotFound                = "AF_RUN_NOT_FOUND"
	CodeNotGitRepo                 = "AF_NOT_GIT_REPO"
	CodeRepoDirty                  = "AF_REPO_DIRTY"
	CodeDependencyMissing          = "AF_DEPENDENCY_MISSING"
	CodeGitNotFound                = "AF_GIT_NOT_FOUND"
	CodeBwrapNotFound              = "AF_BWRAP_NOT_FOUND"
	CodeScannerNotFound            = "AF_SCANNER_NOT_FOUND"
	CodeScanBlocked                = "AF_SCAN_BLOCKED"
	CodePostScanBlocked            = "AF_POST_SCAN_BLOCKED"
	CodeSandboxUnavailable         = "AF_SANDBOX_UNAVAILABLE"
	CodeUnsupportedSandbox         = "AF_UNSUPPORTED_SANDBOX"
	CodeAgentFailed                = "AF_AGENT_FAILED"
	CodeTimeout                    = "AF_TIMEOUT"
	CodeApplyConflict              = "AF_APPLY_CONFLICT"
	CodeActiveRunExists            = "AF_ACTIVE_RUN_EXISTS"
	CodeCancelled                  = "AF_CANCELLED"
	CodeUnauthorized               = "AF_UNAUTHORIZED"
	CodeDaemonNonLoopbackForbidden = "AF_DAEMON_NON_LOOPBACK_FORBIDDEN"
	CodeDiskFull                   = "AF_DISK_FULL"
)

const (
	ExitOK                 = 0
	ExitUsage              = 2
	ExitDependencyMissing  = 4
	ExitSandboxUnavailable = 6
	ExitInternal           = 1
	ExitNotFound           = 3
	ExitSecurityBlocked    = 5
	ExitApplyConflict      = 7
	ExitTimeout            = 8
	ExitCancelled          = 9
)
