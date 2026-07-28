package errorsx

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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

func TestPublicErrorHelpers(t *testing.T) {
	t.Parallel()
	cause := errors.New("cause")
	public := WithDetails(
		Wrap(CodeValidation, "invalid", ExitUsage, cause),
		map[string]any{
			"strings": []string{"short", strings.Repeat("x", 65)},
			"values":  map[string]string{"token": strings.Repeat("y", 65)},
			"number":  7,
		},
	)
	if !errors.Is(public, cause) {
		t.Fatal("public error did not unwrap its cause")
	}
	resolved, ok := IsPublic(fmt.Errorf("wrapped: %w", public))
	if !ok || resolved != public {
		t.Fatalf("public error was not resolved: %#v", resolved)
	}
	if value := WithDetails(nil, nil); value != nil {
		t.Fatalf("nil public error became %#v", value)
	}
	if details := RedactDetails(nil); len(details) != 0 {
		t.Fatalf("nil details became %#v", details)
	}
}

func TestHTTPStatusMappings(t *testing.T) {
	t.Parallel()
	tests := map[string]int{
		CodeUnauthorized:  http.StatusUnauthorized,
		CodeValidation:    http.StatusBadRequest,
		CodeRunNotFound:   http.StatusNotFound,
		CodeApplyConflict: http.StatusConflict,
		CodeTimeout:       http.StatusGatewayTimeout,
		CodeBusy:          http.StatusServiceUnavailable,
		CodeInternal:      http.StatusInternalServerError,
	}
	for code, expected := range tests {
		if status := HTTPStatus(code); status != expected {
			t.Fatalf("status for %s = %d, want %d", code, status, expected)
		}
	}
}
