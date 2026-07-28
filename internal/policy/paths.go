package policy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentfence/agentfence/internal/domain"
	"golang.org/x/sys/unix"
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

func OpenRegularNoSymlinks(root string, relativePath string) (*os.File, error) {
	if _, err := SafeJoin(root, relativePath); err != nil {
		return nil, err
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}
	rootFD, err := unix.Open(resolvedRoot, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open root directory: %w", err)
	}
	currentFD := rootFD
	segments := strings.Split(filepath.ToSlash(relativePath), "/")
	for index, segment := range segments {
		flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW
		if index < len(segments)-1 {
			flags |= unix.O_DIRECTORY
		}
		nextFD, openErr := unix.Openat(currentFD, segment, flags, 0)
		closeErr := unix.Close(currentFD)
		if openErr != nil {
			return nil, fmt.Errorf("open path component: %w", openErr)
		}
		if closeErr != nil {
			_ = unix.Close(nextFD)
			return nil, fmt.Errorf("close path component: %w", closeErr)
		}
		currentFD = nextFD
	}
	file := os.NewFile(uintptr(currentFD), relativePath)
	if file == nil {
		_ = unix.Close(currentFD)
		return nil, fmt.Errorf("open regular file")
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("stat regular file: %w", err)
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, domain.ErrUnsafePath
	}
	return file, nil
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
