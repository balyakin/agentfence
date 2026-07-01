package project

import (
	"path/filepath"

	"github.com/agentfence/agentfence/internal/policy"
)

func safeRepoPath(repoRoot string, relativePath string) (string, error) {
	return policy.SafeJoin(repoRoot, relativePath)
}

func normalizeRelative(path string) string {
	return filepath.ToSlash(path)
}
