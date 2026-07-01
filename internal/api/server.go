package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentfence/agentfence/internal/app"
	"github.com/agentfence/agentfence/internal/config"
	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/errorsx"
	"github.com/agentfence/agentfence/internal/execx"
	"github.com/agentfence/agentfence/internal/ports"
	"github.com/agentfence/agentfence/internal/state"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

type Server struct {
	router       http.Handler
	httpServer   *http.Server
	store        ports.Store
	git          ports.GitRunner
	runService   *app.RunService
	applyService *app.ApplyService
	scanService  *app.ScanService
	queue        *RunQueue
	validator    *validator.Validate
	redactor     *execx.Redactor
	paths        state.Paths
	logger       *slog.Logger
	token        string
	tcpMode      bool
}

type ServerDeps struct {
	Store        ports.Store
	Git          ports.GitRunner
	RunService   *app.RunService
	ApplyService *app.ApplyService
	ScanService  *app.ScanService
	Queue        *RunQueue
	Paths        state.Paths
	Logger       *slog.Logger
	Redactor     *execx.Redactor
	Token        string
	TCPMode      bool
}

func NewServer(deps ServerDeps) (*Server, error) {
	validate, err := config.NewValidator()
	if err != nil {
		return nil, err
	}
	server := &Server{
		store: deps.Store, git: deps.Git, runService: deps.RunService, applyService: deps.ApplyService, scanService: deps.ScanService,
		queue: deps.Queue, validator: validate, redactor: deps.Redactor, paths: deps.Paths, logger: deps.Logger, token: deps.Token, tcpMode: deps.TCPMode,
	}
	server.router = server.routes()
	server.httpServer = &http.Server{
		Handler:           server.router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	return server, nil
}

func (s *Server) Handler() http.Handler {
	return s.router
}

func (s *Server) routes() http.Handler {
	r := chi.NewRouter()
	r.Use(s.requestID)
	r.Use(s.httpLogger)
	r.Use(s.recoverer)
	r.Use(s.bodyLimit)
	r.Use(s.auth)
	r.Use(s.runRateLimit)
	r.Get("/v1/health", s.handleHealth)
	r.Post("/v1/runs", s.handleCreateRun)
	r.Get("/v1/runs", s.handleListRuns)
	r.Get("/v1/runs/{id}", s.handleGetRun)
	r.Post("/v1/runs/{id}/cancel", s.handleCancelRun)
	r.Get("/v1/runs/{id}/diff", s.handleDiff)
	r.Post("/v1/runs/{id}/apply", s.handleApply)
	r.Post("/v1/scan", s.handleScan)
	return r
}

func (s *Server) Serve(ctx context.Context, network string, address string) error {
	listener, err := listener(network, address, s.paths)
	if err != nil {
		return err
	}
	defer func() {
		if err := listener.Close(); err != nil && s.logger != nil {
			s.logger.ErrorContext(ctx, "close listener failed", slog.Any("error", err))
		}
	}()
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.httpServer.Serve(listener)
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown daemon: %w", err)
		}
		return nil
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func listener(network string, address string, paths state.Paths) (net.Listener, error) {
	if network == "tcp" {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return nil, errorsx.Wrap(errorsx.CodeDaemonNonLoopbackForbidden, "invalid tcp listen address", errorsx.ExitUsage, err)
		}
		if host != "127.0.0.1" {
			return nil, errorsx.Wrap(errorsx.CodeDaemonNonLoopbackForbidden, "tcp daemon must bind to 127.0.0.1", errorsx.ExitUsage, nil)
		}
		return net.Listen("tcp", address)
	}
	socketPath := address
	if socketPath == "" {
		socketPath = filepath.Join(paths.StateDir, "agentfence.sock")
	}
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove stale socket: %w", err)
	}
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		closeErr := ln.Close()
		if closeErr != nil {
			return nil, fmt.Errorf("chmod unix socket: %w; close socket: %w", err, closeErr)
		}
		return nil, fmt.Errorf("chmod unix socket: %w", err)
	}
	return ln, nil
}

func GenerateToken() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func WriteTokenFile(path string, token string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create token directory: %w", err)
	}
	return os.WriteFile(path, []byte(token+"\n"), 0o600)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(value); err != nil {
		http.Error(w, `{"code":"AF_INTERNAL","message":"encode response failed"}`, http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, err error) {
	public, ok := errorsx.IsPublic(err)
	if !ok {
		public = errorsx.Wrap(errorsx.CodeInternal, "internal error", errorsx.ExitInternal, err)
	}
	writeJSON(w, errorsx.HTTPStatus(public.Code), public)
}

func requestToRun(runID string, req CreateRunRequest) domain.RunRequest {
	return domain.RunRequest{
		RunID: runID, RepoPath: req.RepoPath, Agent: req.Agent, Task: strings.TrimSpace(req.Task), AgentArgs: req.AgentArgs,
		TimeoutSeconds: req.TimeoutSeconds, Network: req.Network, IncludeUntracked: req.IncludeUntracked, IncludeDirty: req.IncludeDirty, AllowSoftMode: req.AllowSoftMode,
	}
}
