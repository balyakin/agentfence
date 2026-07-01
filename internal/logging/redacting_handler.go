package logging

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/agentfence/agentfence/internal/execx"
)

type RedactingHandler struct {
	next     slog.Handler
	redactor *execx.Redactor
}

func NewRedactingHandler(next slog.Handler, redactor *execx.Redactor) *RedactingHandler {
	if redactor == nil {
		redactor = execx.NewRedactor()
	}
	return &RedactingHandler{next: next, redactor: redactor}
}

func (h *RedactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *RedactingHandler) Handle(ctx context.Context, record slog.Record) error {
	copied := slog.NewRecord(record.Time, record.Level, h.redactor.RedactString(record.Message), record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		copied.AddAttrs(h.redactAttr(attr))
		return true
	})
	return h.next.Handle(ctx, copied)
}

func (h *RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		redacted = append(redacted, h.redactAttr(attr))
	}
	return &RedactingHandler{next: h.next.WithAttrs(redacted), redactor: h.redactor}
}

func (h *RedactingHandler) WithGroup(name string) slog.Handler {
	return &RedactingHandler{next: h.next.WithGroup(name), redactor: h.redactor}
}

func (h *RedactingHandler) redactAttr(attr slog.Attr) slog.Attr {
	switch attr.Value.Kind() {
	case slog.KindString:
		return slog.String(attr.Key, h.redactor.RedactString(attr.Value.String()))
	case slog.KindGroup:
		group := attr.Value.Group()
		redacted := make([]slog.Attr, 0, len(group))
		for _, child := range group {
			redacted = append(redacted, h.redactAttr(child))
		}
		return slog.Group(attr.Key, attrsToAny(redacted)...)
	case slog.KindAny:
		return slog.String(attr.Key, h.redactor.RedactString(fmt.Sprint(attr.Value.Any())))
	default:
		return attr
	}
}

func attrsToAny(attrs []slog.Attr) []any {
	values := make([]any, len(attrs))
	for i, attr := range attrs {
		values[i] = attr
	}
	return values
}
