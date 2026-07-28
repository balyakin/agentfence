package logging

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/agentfence/agentfence/internal/config"
	"github.com/agentfence/agentfence/internal/execx"
)

func TestRedactingHandlerRedactsAnyAttr(t *testing.T) {
	t.Parallel()
	secret := "abcdefghijklmnopqrstuvwxyz123456"
	redactor := execx.NewRedactor()
	redactor.RegisterSecret(secret)
	var buf bytes.Buffer
	logger := slog.New(NewRedactingHandler(slog.NewTextHandler(&buf, nil), redactor))
	logger.Error("failed", slog.Any("error", errors.New("token="+secret)))
	if strings.Contains(buf.String(), secret) {
		t.Fatalf("secret leaked in any attr: %q", buf.String())
	}
}

func TestLoggerRedactsMessageAndNestedAttributes(t *testing.T) {
	t.Parallel()
	secret := "abcdefghijklmnopqrstuvwxyz123456"
	redactor := execx.NewRedactor()
	redactor.RegisterSecret(secret)
	var output bytes.Buffer
	logger := New(config.LoggingConfig{Level: "debug", JSON: true}, &output, redactor)
	logger.With("bound", secret).WithGroup("group").Info(
		"message "+secret,
		slog.String("string", secret),
		slog.Group("nested", slog.String("value", secret)),
		slog.Int("number", 7),
	)
	if strings.Contains(output.String(), secret) {
		t.Fatalf("secret leaked from structured log: %q", output.String())
	}
}

func TestRedactingHandlerDelegatesEnabled(t *testing.T) {
	t.Parallel()
	handler := NewRedactingHandler(slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}), nil)
	if handler.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("info unexpectedly enabled")
	}
	if !handler.Enabled(context.Background(), slog.LevelError) {
		t.Fatal("error unexpectedly disabled")
	}
}
