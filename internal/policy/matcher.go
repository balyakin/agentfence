package policy

import (
	"fmt"
	"path/filepath"

	"github.com/bmatcuk/doublestar/v4"
)

type Matcher struct {
	include []string
	exclude []string
}

func NewMatcher(include []string, exclude []string) (*Matcher, error) {
	if len(include) == 0 {
		include = []string{"**"}
	}
	for _, pattern := range append(append([]string{}, include...), exclude...) {
		if pattern == "" {
			return nil, fmt.Errorf("empty policy pattern")
		}
		if _, err := doublestar.Match(pattern, "x"); err != nil {
			return nil, fmt.Errorf("invalid policy pattern %q: %w", pattern, err)
		}
	}
	return &Matcher{include: append([]string{}, include...), exclude: append([]string{}, exclude...)}, nil
}

func (m *Matcher) Allowed(relativePath string) (bool, error) {
	path := filepath.ToSlash(relativePath)
	for _, pattern := range m.exclude {
		matched, err := doublestar.Match(pattern, path)
		if err != nil {
			return false, fmt.Errorf("match exclude pattern: %w", err)
		}
		if matched {
			return false, nil
		}
	}
	for _, pattern := range m.include {
		matched, err := doublestar.Match(pattern, path)
		if err != nil {
			return false, fmt.Errorf("match include pattern: %w", err)
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}
