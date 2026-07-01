package state

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

type Paths struct {
	StateDir string
	CacheDir string
	RunsDir  string
	LocksDir string
	DBPath   string
}

func Resolve(ctx context.Context, stateDir string, cacheDir string) (Paths, error) {
	if err := ctx.Err(); err != nil {
		return Paths{}, err
	}
	resolvedState, err := resolveStateDir(stateDir)
	if err != nil {
		return Paths{}, err
	}
	resolvedCache, err := resolveCacheDir(cacheDir)
	if err != nil {
		return Paths{}, err
	}
	paths := Paths{
		StateDir: resolvedState,
		CacheDir: resolvedCache,
		RunsDir:  filepath.Join(resolvedCache, "runs"),
		LocksDir: filepath.Join(resolvedState, "locks"),
		DBPath:   filepath.Join(resolvedState, "agentfence.db"),
	}
	for _, dir := range []string{paths.StateDir, paths.CacheDir, paths.RunsDir, paths.LocksDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return Paths{}, fmt.Errorf("create state directory: %w", err)
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return Paths{}, fmt.Errorf("chmod state directory: %w", err)
		}
	}
	return paths, nil
}

func (p Paths) RunDir(runID string) string {
	return filepath.Join(p.RunsDir, runID)
}

func (p Paths) LockPath(repoPath string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(repoPath)))
	return filepath.Join(p.LocksDir, hex.EncodeToString(sum[:])+".lock")
}

func RepoHash(repoPath string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(repoPath)))
	return hex.EncodeToString(sum[:8])
}

func resolveStateDir(configured string) (string, error) {
	if configured != "" {
		return filepath.Abs(configured)
	}
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Abs(filepath.Join(xdg, "agentfence"))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Abs(filepath.Join(home, ".local", "state", "agentfence"))
}

func resolveCacheDir(configured string) (string, error) {
	if configured != "" {
		return filepath.Abs(configured)
	}
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Abs(filepath.Join(xdg, "agentfence"))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Abs(filepath.Join(home, ".cache", "agentfence"))
}
