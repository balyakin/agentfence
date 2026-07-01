package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/agentfence/agentfence/internal/config"
	"github.com/agentfence/agentfence/internal/execx"
)

func New(cfg config.LoggingConfig, out io.Writer, redactor *execx.Redactor) *slog.Logger {
	if out == nil {
		out = os.Stderr
	}
	level := slog.LevelInfo
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if cfg.JSON {
		handler = slog.NewJSONHandler(out, opts)
	} else {
		handler = slog.NewTextHandler(out, opts)
	}
	return slog.New(NewRedactingHandler(handler, redactor))
}
