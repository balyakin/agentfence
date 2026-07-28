package workspace

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/agentfence/agentfence/internal/config"
)

const (
	generatedEnvPath    = ".env"
	fakeDatabaseName    = "agentfence"
	fakeDatabaseHost    = "127.0.0.1:1"
	fakeHTTPURL         = "http://127.0.0.1:1"
	fakePort            = "1"
	fakeBool            = "false"
	fakeEnvironmentName = "test"
	fakeValue           = "agentfence-placeholder"
)

var fakeDatabaseURL = "postgres://" + fakeDatabaseName + ":" + fakeDatabaseName + "@" + fakeDatabaseHost +
	"/agentfence?sslmode=disable"

var envKeyPattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

type sanitizedEnvResult struct {
	Env              []string
	Keys             []string
	SourceFiles      []string
	IgnoredPatchPath string
}

func buildSanitizedEnv(repoRoot string, shadowRoot string, cfg config.SanitizedEnvConfig) (sanitizedEnvResult, error) {
	if !cfg.Enabled {
		return sanitizedEnvResult{}, nil
	}
	keys, sources, err := collectEnvKeys(repoRoot, cfg)
	if err != nil {
		return sanitizedEnvResult{}, err
	}
	if len(keys) == 0 {
		return sanitizedEnvResult{SourceFiles: sources}, nil
	}
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+placeholderValue(key))
	}
	envPath := filepath.Join(shadowRoot, generatedEnvPath)
	if err := writeEnvFile(envPath, env); err != nil {
		return sanitizedEnvResult{}, err
	}
	return sanitizedEnvResult{
		Env:              env,
		Keys:             keys,
		SourceFiles:      sources,
		IgnoredPatchPath: generatedEnvPath,
	}, nil
}

func collectEnvKeys(repoRoot string, cfg config.SanitizedEnvConfig) ([]string, []string, error) {
	keySet := map[string]struct{}{}
	sources := make([]string, 0, len(cfg.ExampleFiles))
	for _, source := range cfg.ExampleFiles {
		sourcePath, err := safeEnvSourcePath(repoRoot, source)
		if err != nil {
			return nil, nil, err
		}
		keys, err := collectEnvFileKeys(sourcePath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, nil, err
		}
		sources = append(sources, filepath.ToSlash(filepath.Clean(source)))
		for _, key := range keys {
			if validEnvKey(key) && !reservedEnvKey(key) {
				keySet[key] = struct{}{}
			}
		}
	}
	for _, key := range cfg.ExtraKeys {
		if validEnvKey(key) && !reservedEnvKey(key) {
			keySet[key] = struct{}{}
		}
	}
	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, sources, nil
}

func collectEnvFileKeys(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	keys := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, ok := envKeyFromLine(scanner.Text())
		if ok {
			keys = append(keys, key)
		}
	}
	scanErr := scanner.Err()
	closeErr := file.Close()
	if scanErr != nil && closeErr != nil {
		return nil, fmt.Errorf("scan env source: %w; close env source: %w", scanErr, closeErr)
	}
	if scanErr != nil {
		return nil, fmt.Errorf("scan env source: %w", scanErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close env source: %w", closeErr)
	}
	return keys, nil
}

func safeEnvSourcePath(repoRoot string, source string) (string, error) {
	if source == "" || filepath.IsAbs(source) || containsParentSegment(source) {
		return "", fmt.Errorf("unsafe env source path %q", source)
	}
	clean := filepath.Clean(source)
	if clean == "." {
		return "", fmt.Errorf("unsafe env source path %q", source)
	}
	base := strings.ToLower(filepath.Base(clean))
	if base == generatedEnvPath || !safeEnvSourceName(base) {
		return "", fmt.Errorf("unsafe env source path %q", source)
	}
	path := filepath.Join(repoRoot, clean)
	info, err := lstatEnvSourcePath(repoRoot, clean)
	if err != nil {
		if os.IsNotExist(err) {
			return path, nil
		}
		return "", fmt.Errorf("stat env source: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("env source must not be a symlink: %s", clean)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("env source must be a regular file: %s", clean)
	}
	return path, nil
}

func envKeyFromLine(line string) (string, bool) {
	text := strings.TrimSpace(line)
	if text == "" || strings.HasPrefix(text, "#") {
		return "", false
	}
	if strings.HasPrefix(text, "export ") {
		text = strings.TrimSpace(strings.TrimPrefix(text, "export "))
	}
	key := text
	if before, _, ok := strings.Cut(text, "="); ok {
		key = before
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", false
	}
	return key, true
}

func validEnvKey(key string) bool {
	return envKeyPattern.MatchString(key)
}

func reservedEnvKey(key string) bool {
	switch key {
	case "PATH", "HOME", "PWD", "SHELL", "USER", "LOGNAME", "SSH_AUTH_SOCK", "AGENTFENCE_SANDBOX":
		return true
	default:
		return false
	}
}

func placeholderValue(key string) string {
	switch {
	case key == "DATABASE_URL":
		return fakeDatabaseURL
	case strings.HasSuffix(key, "_DATABASE_URL"):
		return fakeDatabaseURL
	case key == "POSTGRES_URL":
		return fakeDatabaseURL
	case strings.HasSuffix(key, "_POSTGRES_URL"):
		return fakeDatabaseURL
	case key == "POSTGRES_DSN":
		return fakeDatabaseURL
	case strings.HasSuffix(key, "_POSTGRES_DSN"):
		return fakeDatabaseURL
	case key == "PG_DSN":
		return fakeDatabaseURL
	case strings.HasSuffix(key, "_PG_DSN"):
		return fakeDatabaseURL
	case key == "SQL_DSN":
		return fakeDatabaseURL
	case strings.HasSuffix(key, "_SQL_DSN"):
		return fakeDatabaseURL
	case strings.HasSuffix(key, "_PORT"):
		return fakePort
	case strings.HasSuffix(key, "_URL"):
		return fakeHTTPURL
	case strings.HasPrefix(key, "ENABLE_"):
		return fakeBool
	case strings.HasPrefix(key, "DISABLE_"):
		return fakeBool
	case strings.HasPrefix(key, "IS_"):
		return fakeBool
	case strings.HasPrefix(key, "HAS_"):
		return fakeBool
	case key == "NODE_ENV" || key == "APP_ENV" || key == "RAILS_ENV" || key == "ENV":
		return fakeEnvironmentName
	default:
		return fakeValue
	}
}

func writeEnvFile(path string, env []string) error {
	values := append([]string{}, env...)
	sort.Strings(values)
	data := strings.Join(values, "\n") + "\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		return fmt.Errorf("write sanitized env: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod sanitized env: %w", err)
	}
	return nil
}

func containsParentSegment(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func lstatEnvSourcePath(repoRoot string, cleanSource string) (os.FileInfo, error) {
	current := repoRoot
	parts := strings.Split(filepath.ToSlash(cleanSource), "/")
	for index, part := range parts {
		current = filepath.Join(current, filepath.FromSlash(part))
		info, err := os.Lstat(current)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("env source must not contain a symlink: %s", cleanSource)
		}
		if index < len(parts)-1 && !info.IsDir() {
			return nil, fmt.Errorf("env source parent must be a directory: %s", cleanSource)
		}
		if index == len(parts)-1 {
			return info, nil
		}
	}
	return nil, os.ErrNotExist
}

func safeEnvSourceName(name string) bool {
	return strings.Contains(name, "example") || strings.Contains(name, "template") || strings.Contains(name, "sample")
}
