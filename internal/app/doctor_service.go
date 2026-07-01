package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"

	"github.com/agentfence/agentfence/internal/config"
	"github.com/agentfence/agentfence/internal/errorsx"
	"github.com/agentfence/agentfence/internal/state"
)

type DoctorService struct {
	paths state.Paths
}

type DoctorCheck struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	Code    string `json:"code,omitempty"`
}

type DoctorResult struct {
	Checks   []DoctorCheck `json:"checks"`
	ExitCode int           `json:"exit_code"`
}

func NewDoctorService(paths state.Paths) *DoctorService {
	return &DoctorService{paths: paths}
}

func (s *DoctorService) Check(ctx context.Context, cfg config.Config) DoctorResult {
	result := DoctorResult{ExitCode: errorsx.ExitOK}
	result.add("config", config.Validate(cfg) == nil, errorsx.CodeConfigInvalid)
	result.add("git", commandOK(ctx, "git", "--version"), errorsx.CodeGitNotFound)
	result.add("gitleaks", commandOK(ctx, "gitleaks", "version"), errorsx.CodeScannerNotFound)
	for _, engine := range cfg.Scan.Engines {
		if engine == "trufflehog" {
			result.add("trufflehog", commandOK(ctx, "trufflehog", "--version"), errorsx.CodeScannerNotFound)
		}
	}
	if cfg.Sandbox.Mode == "bubblewrap" && runtime.GOOS == "linux" {
		result.add("bubblewrap", commandOK(ctx, "bwrap", "--version"), errorsx.CodeBwrapNotFound)
		result.add("bubblewrap capability", commandOK(ctx, "bwrap", "--unshare-all", "--die-with-parent", "--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp", "true"), errorsx.CodeSandboxUnavailable)
	}
	for _, dir := range []string{s.paths.StateDir, s.paths.CacheDir, s.paths.RunsDir, s.paths.LocksDir} {
		ok := dirPermOK(dir)
		result.add("dir "+dir, ok, errorsx.CodeInternal)
	}
	result.add("sqlite db writable", fileWritable(s.paths.DBPath), errorsx.CodeInternal)
	if freeBytes(s.paths.CacheDir) < 500*1024*1024 {
		result.add("cache free space", false, errorsx.CodeDiskFull)
	}
	for _, check := range result.Checks {
		if check.OK {
			continue
		}
		switch check.Code {
		case errorsx.CodeBwrapNotFound, errorsx.CodeSandboxUnavailable:
			if result.ExitCode == errorsx.ExitOK {
				result.ExitCode = errorsx.ExitSandboxUnavailable
			}
		case errorsx.CodeGitNotFound, errorsx.CodeScannerNotFound:
			if result.ExitCode == errorsx.ExitOK {
				result.ExitCode = errorsx.ExitDependencyMissing
			}
		case errorsx.CodeConfigInvalid:
			if result.ExitCode == errorsx.ExitOK {
				result.ExitCode = errorsx.ExitUsage
			}
		default:
			if result.ExitCode == errorsx.ExitOK {
				result.ExitCode = errorsx.ExitInternal
			}
		}
	}
	return result
}

func (r *DoctorResult) add(name string, ok bool, code string) {
	check := DoctorCheck{Name: name, OK: ok, Code: code}
	if !ok {
		check.Message = "check failed"
	}
	r.Checks = append(r.Checks, check)
}

func commandOK(ctx context.Context, executable string, args ...string) bool {
	if _, err := exec.LookPath(executable); err != nil {
		return false
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	return cmd.Run() == nil
}

func dirPermOK(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil {
		return false
	}
	return info.IsDir() && info.Mode().Perm() == 0o700
}

func freeBytes(dir string) uint64 {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(dir, &stat); err != nil {
		return 0
	}
	if stat.Bsize <= 0 {
		return 0
	}
	return stat.Bavail * uint64(stat.Bsize) // #nosec G115 -- syscall returned a positive block size.
}

func fileWritable(path string) bool {
	if path == "" {
		return false
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return false
	}
	if err := file.Close(); err != nil {
		return false
	}
	return os.Chmod(path, 0o600) == nil
}

func (r DoctorResult) Error() error {
	if r.ExitCode == errorsx.ExitOK {
		return nil
	}
	return fmt.Errorf("doctor checks failed")
}
