package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/state"
	"github.com/agentfence/agentfence/internal/testutil"
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

func TestUnixListenerRefusesRegularFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "daemon.sock")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	_, err := listener("unix", path, state.Paths{})
	if err == nil {
		t.Fatalf("regular file was replaced")
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read preserved file: %v", readErr)
	}
	if string(data) != "keep" {
		t.Fatalf("regular file changed: %q", data)
	}
}

func TestUnixListenerRefusesLiveSocket(t *testing.T) {
	t.Parallel()
	dir, err := os.MkdirTemp("/tmp", "agentfence-socket-")
	if err != nil {
		t.Fatalf("create socket dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	path := filepath.Join(dir, "daemon.sock")
	active, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen active: %v", err)
	}
	defer func() {
		_ = active.Close()
	}()
	_, err = listener("unix", path, state.Paths{})
	if err == nil {
		t.Fatalf("live socket was replaced")
	}
}

func TestWriteTokenFileReplacesSymlinkWithoutFollowing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(target, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	if err := WriteTokenFile(path, "new-token"); err != nil {
		t.Fatalf("write token: %v", err)
	}
	targetData, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(targetData) != "keep" {
		t.Fatalf("symlink target changed: %q", targetData)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat token: %v", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("token mode=%v", info.Mode())
	}
}

func TestRateLimitUsesJSONForScan(t *testing.T) {
	t.Parallel()
	server, err := NewServer(ServerDeps{})
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	handler := server.runRateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}))
	var recorder *httptest.ResponseRecorder
	for count := 0; count < 4; count++ {
		recorder = httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/v1/scan", nil)
		handler.ServeHTTP(recorder, request)
	}
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", recorder.Code)
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["code"] == nil {
		t.Fatalf("error code missing: %s", recorder.Body.String())
	}
}

func TestListRunsUsesSnakeCaseDTO(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeStore()
	now := time.Now().UTC()
	if err := store.CreateRun(context.Background(), domain.Run{
		ID: "run", RepoPath: "/repo", AgentName: "agent", Status: domain.RunStatusSucceeded,
		TaskRedacted: "[redacted]", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	server, err := NewServer(ServerDeps{Store: store})
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/runs", nil)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response []map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response) != 1 || response[0]["repo_path"] != "/repo" || response[0]["RepoPath"] != nil {
		t.Fatalf("unexpected response: %#v", response)
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
