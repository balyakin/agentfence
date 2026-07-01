package errorsx

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestPublicErrorDoesNotExposeCause(t *testing.T) {
	t.Parallel()
	err := Wrap(CodeInternal, "failed", ExitInternal, errors.New("raw-secret-value"))
	if strings.Contains(err.Error(), "raw-secret-value") {
		t.Fatalf("cause leaked through Error()")
	}
}

func TestRedactDetailsNestedValues(t *testing.T) {
	t.Parallel()
	secret := strings.Repeat("s", 80)
	details := RedactDetails(map[string]any{
		"token": secret,
		"nested": map[string]any{
			"token": secret,
		},
		"list":  []any{secret, map[string]any{"token": secret}},
		"bytes": []byte(secret),
	})
	data, err := json.Marshal(details)
	if err != nil {
		t.Fatalf("marshal details: %v", err)
	}
	if strings.Contains(string(data), secret[:16]) {
		t.Fatalf("secret prefix leaked: %s", data)
	}
	if strings.Count(string(data), "[redacted]") != 5 {
		t.Fatalf("expected nested redactions, got %s", data)
	}
}
