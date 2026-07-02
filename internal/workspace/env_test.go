package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentfence/agentfence/internal/config"
)

func TestBuildSanitizedEnvIgnoresSourceValues(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	shadowRoot := t.TempDir()
	sourcePath := filepath.Join(repoRoot, ".env.example")
	data := []byte("DATABASE_URL=postgres://real-user:real-pass@db/prod\nAPI_URL=https://real.example\nPOSTGRES_PORT=5432\nAPI_KEY=real-secret\n")
	if err := os.WriteFile(sourcePath, data, 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	result, err := buildSanitizedEnv(repoRoot, shadowRoot, config.SanitizedEnvConfig{
		Enabled:      true,
		ExampleFiles: []string{".env.example"},
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	envPath := filepath.Join(shadowRoot, ".env")
	envData, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read generated env: %v", err)
	}
	envText := string(envData)
	if strings.Contains(envText, "real-user") || strings.Contains(envText, "real-secret") {
		t.Fatalf("source value leaked: %s", envText)
	}
	if !strings.Contains(envText, "DATABASE_URL=postgres://agentfence:agentfence@127.0.0.1:1/agentfence?sslmode=disable") {
		t.Fatalf("missing fake database url: %s", envText)
	}
	if !strings.Contains(envText, "API_URL=http://127.0.0.1:1") {
		t.Fatalf("missing fake api url: %s", envText)
	}
	if !strings.Contains(envText, "POSTGRES_PORT=1") {
		t.Fatalf("missing fake postgres port: %s", envText)
	}
	if !strings.Contains(envText, "API_KEY=agentfence-placeholder") {
		t.Fatalf("missing placeholder api key: %s", envText)
	}
	if len(result.Env) != 4 {
		t.Fatalf("env count=%d", len(result.Env))
	}
}

func TestPlaceholderValueUsesPostgresForBareDsnKeys(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"POSTGRES_URL", "POSTGRES_DSN", "PG_DSN", "SQL_DSN"} {
		if value := placeholderValue(key); value != fakeDatabaseURL {
			t.Fatalf("%s=%s", key, value)
		}
	}
}

func TestBuildSanitizedEnvRejectsRuntimeEnvSource(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	shadowRoot := t.TempDir()
	_, err := buildSanitizedEnv(repoRoot, shadowRoot, config.SanitizedEnvConfig{
		Enabled:      true,
		ExampleFiles: []string{".env"},
	})
	if err == nil {
		t.Fatalf("expected unsafe source error")
	}
}

func TestBuildSanitizedEnvRejectsParentEnvSource(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	shadowRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, ".env.example"), []byte("DATABASE_URL=\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	_, err := buildSanitizedEnv(repoRoot, shadowRoot, config.SanitizedEnvConfig{
		Enabled:      true,
		ExampleFiles: []string{"config/../.env.example"},
	})
	if err == nil {
		t.Fatalf("expected unsafe source error")
	}
}

func TestBuildSanitizedEnvRejectsSymlinkParentSource(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	shadowRoot := t.TempDir()
	outsideRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideRoot, ".env.example"), []byte("REAL_SECRET_KEY=\n"), 0o600); err != nil {
		t.Fatalf("write outside source: %v", err)
	}
	if err := os.Symlink(outsideRoot, filepath.Join(repoRoot, "configs")); err != nil {
		t.Fatalf("symlink parent: %v", err)
	}
	_, err := buildSanitizedEnv(repoRoot, shadowRoot, config.SanitizedEnvConfig{
		Enabled:      true,
		ExampleFiles: []string{"configs/.env.example"},
	})
	if err == nil {
		t.Fatalf("expected symlink parent source error")
	}
}

func TestBuildSanitizedEnvSkipsReservedKeys(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	shadowRoot := t.TempDir()
	sourcePath := filepath.Join(repoRoot, ".env.example")
	if err := os.WriteFile(sourcePath, []byte("PATH=/tmp/bin\nNODE_ENV=development\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	result, err := buildSanitizedEnv(repoRoot, shadowRoot, config.SanitizedEnvConfig{
		Enabled:      true,
		ExampleFiles: []string{".env.example"},
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(result.Env) != 1 {
		t.Fatalf("env=%v", result.Env)
	}
	if result.Env[0] != "NODE_ENV=test" {
		t.Fatalf("env=%v", result.Env)
	}
}

func TestBuildSanitizedEnvSkipsReservedExtraKeys(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	shadowRoot := t.TempDir()
	result, err := buildSanitizedEnv(repoRoot, shadowRoot, config.SanitizedEnvConfig{
		Enabled:   true,
		ExtraKeys: []string{"PATH", "DATABASE_URL"},
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(result.Env) != 1 {
		t.Fatalf("env=%v", result.Env)
	}
	if !strings.HasPrefix(result.Env[0], "DATABASE_URL=") {
		t.Fatalf("env=%v", result.Env)
	}
}

func TestBuildSanitizedEnvRejectsSymlinkSource(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	shadowRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, ".env"), []byte("DATABASE_URL=real\n"), 0o600); err != nil {
		t.Fatalf("write env: %v", err)
	}
	if err := os.Symlink(filepath.Join(repoRoot, ".env"), filepath.Join(repoRoot, ".env.example")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	_, err := buildSanitizedEnv(repoRoot, shadowRoot, config.SanitizedEnvConfig{
		Enabled:      true,
		ExampleFiles: []string{".env.example"},
	})
	if err == nil {
		t.Fatalf("expected symlink source error")
	}
}
