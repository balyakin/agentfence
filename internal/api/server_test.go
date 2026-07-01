package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthUnixNoAuth(t *testing.T) {
	t.Parallel()
	server, err := NewServer(ServerDeps{})
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestTCPAuthRequired(t *testing.T) {
	t.Parallel()
	server, err := NewServer(ServerDeps{TCPMode: true, Token: "secret"})
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestHandleApplyRejectsLatest(t *testing.T) {
	t.Parallel()
	server, err := NewServer(ServerDeps{})
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/latest/apply", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rec.Code)
	}
}
