package domain

import (
	"strings"
	"testing"
)

func TestRunStatusTerminal(t *testing.T) {
	t.Parallel()
	if RunStatusRunning.IsTerminal() {
		t.Fatalf("running must not be terminal")
	}
	if !RunStatusSucceeded.IsTerminal() {
		t.Fatalf("succeeded must be terminal")
	}
}

func TestFindingHelpers(t *testing.T) {
	t.Parallel()
	if RedactSecret("short") != "****" {
		t.Fatal("short secret was not fully redacted")
	}
	longSecret := "long-" + strings.Repeat("value", 4)
	if RedactSecret(longSecret) != "...alue" {
		t.Fatalf("unexpected long secret redaction: %q", RedactSecret(longSecret))
	}
	if SecretSHA256("value") == SecretSHA256("other") {
		t.Fatal("different secrets share a hash")
	}
	findings := []Finding{{Severity: " HIGH "}}
	if !FindingsBlocked(findings, []string{"high"}) {
		t.Fatal("blocking severity was not matched")
	}
	if FindingsBlocked(findings, []string{"critical"}) {
		t.Fatal("non-blocking severity was blocked")
	}
}

func TestRunStatusActive(t *testing.T) {
	t.Parallel()
	if !RunStatusApplying.IsActive() {
		t.Fatal("applying status must be active")
	}
	if RunStatusFailed.IsActive() {
		t.Fatal("failed status must not be active")
	}
	if RunStatus("unknown").IsTerminal() {
		t.Fatal("unknown status must not be terminal")
	}
}
