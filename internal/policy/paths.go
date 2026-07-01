package policy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentfence/agentfence/internal/domain"
)

func SafeJoin(root string, relativePath string) (string, error) {
	if relativePath == "" {
		return "", domain.ErrUnsafePath
	}
	slashPath := filepath.ToSlash(relativePath)
	if strings.ContainsRune(slashPath, '\x00') {
		return "", domain.ErrUnsafePath
	}
	if !filepath.IsLocal(slashPath) {
		return "", domain.ErrUnsafePath
	}
	for _, segment := range strings.Split(slashPath, "/") {
		if strings.EqualFold(segment, ".git") {
			return "", domain.ErrUnsafePath
		}
	}
	fullPath := filepath.Join(root, filepath.FromSlash(slashPath))
	cleanRoot := filepath.Clean(root)
	relativeToRoot, err := filepath.Rel(cleanRoot, fullPath)
	if err != nil {
		return "", fmt.Errorf("calculate relative path: %w", err)
	}
	if relativeToRoot == ".." || strings.HasPrefix(filepath.ToSlash(relativeToRoot), "../") {
		return "", domain.ErrUnsafePath
	}
	return fullPath, nil
}

func ValidateSymlinkInside(root string, path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("lstat path: %w", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return nil
	}
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve root symlink: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolve symlink target: %w", err)
	}
	rootClean := filepath.Clean(rootResolved)
	rel, err := filepath.Rel(rootClean, filepath.Clean(resolved))
	if err != nil {
		return fmt.Errorf("calculate symlink relative path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(filepath.ToSlash(rel), "../") || filepath.IsAbs(rel) {
		return domain.ErrUnsafePath
	}
	return nil
}
