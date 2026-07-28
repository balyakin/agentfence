package api

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/agentfence/agentfence/internal/errorsx"
	"golang.org/x/time/rate"
)

type contextKey string

const requestIDKey contextKey = "request_id"

func (s *Server) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strconv.FormatInt(time.Now().UnixNano(), 36)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

func (s *Server) httpLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(recorder, r)
		if s.logger != nil {
			requestID := ""
			if value, ok := r.Context().Value(requestIDKey).(string); ok {
				requestID = value
			}
			s.logger.InfoContext(r.Context(), "http request",
				slog.String("request_id", requestID),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", recorder.status),
				slog.Int64("latency_ms", time.Since(start).Milliseconds()),
			)
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (s *Server) bodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.tcpMode {
			header := r.Header.Get("Authorization")
			want := "Bearer " + s.token
			if subtle.ConstantTimeCompare([]byte(header), []byte(want)) != 1 {
				writeError(w, errorsx.Wrap(errorsx.CodeUnauthorized, "unauthorized", errorsx.ExitUsage, nil))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				if recovered == http.ErrAbortHandler {
					panic(recovered)
				}
				message := fmt.Sprint(recovered)
				stack := string(debug.Stack())
				if s.redactor != nil {
					message = s.redactor.RedactString(message)
					stack = s.redactor.RedactString(stack)
				}
				if s.logger != nil {
					s.logger.ErrorContext(
						r.Context(),
						"http handler panic",
						slog.String("panic", message),
						slog.String("stack", stack),
					)
				}
				writeError(w, errorsx.Wrap(errorsx.CodeInternal, "internal error", errorsx.ExitInternal, nil))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) runRateLimit(next http.Handler) http.Handler {
	limiter := rate.NewLimiter(rate.Every(time.Minute), 3)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isWorkRequest := r.Method == http.MethodPost &&
			(r.URL.Path == "/v1/runs" || r.URL.Path == "/v1/scan")
		if isWorkRequest {
			if !limiter.Allow() {
				writeError(w, errorsx.Wrap(errorsx.CodeBusy, "rate limited", errorsx.ExitInternal, nil))
				return
			}
		}
		if r.Method == http.MethodPost && r.URL.Path == "/v1/scan" {
			select {
			case s.scanSlots <- struct{}{}:
				defer func() {
					<-s.scanSlots
				}()
			default:
				writeError(w, errorsx.Wrap(errorsx.CodeBusy, "scan already in progress", errorsx.ExitInternal, nil))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
