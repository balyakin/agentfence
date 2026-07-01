package state

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type CleanupCandidate struct {
	RunID  string
	RunDir string
}

type CleanupResult struct {
	Deleted []string `json:"deleted"`
	Skipped []string `json:"skipped"`
}

func CleanupRunDirs(ctx context.Context, paths Paths, candidates []CleanupCandidate, dryRun bool) (CleanupResult, error) {
	var result CleanupResult
	cacheRoot := filepath.Clean(paths.CacheDir)
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		runDir := filepath.Clean(candidate.RunDir)
		rel, err := filepath.Rel(cacheRoot, runDir)
		if err != nil {
			return result, fmt.Errorf("calculate cleanup relative path: %w", err)
		}
		if rel == ".." || strings.HasPrefix(filepath.ToSlash(rel), "../") || filepath.IsAbs(rel) {
			result.Skipped = append(result.Skipped, candidate.RunDir)
			continue
		}
		if _, err := os.Stat(runDir); err != nil {
			if os.IsNotExist(err) {
				result.Skipped = append(result.Skipped, candidate.RunDir)
				continue
			}
			return result, fmt.Errorf("stat cleanup path: %w", err)
		}
		if !dryRun {
			if err := os.RemoveAll(runDir); err != nil {
				return result, fmt.Errorf("remove run directory: %w", err)
			}
		}
		result.Deleted = append(result.Deleted, runDir)
	}
	return result, nil
}
