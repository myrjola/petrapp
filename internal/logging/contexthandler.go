package logging

import (
	"context"
	"fmt"
	"log/slog"
)

type contextKey string

const slogAttrs contextKey = "slogAttrs"

type ContextHandler struct {
	handler slog.Handler
}

// NewContextHandler constructs a ContextHandler that adds new [slog.Attr] to the log messages from [context.Context]
// to the underlying [slog.Handler].
func NewContextHandler(h slog.Handler) *ContextHandler {
	return &ContextHandler{handler: h}
}

// Enabled reports whether the handler handles records at the given level.
// It delegates to the underlying handler.
func (h *ContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

// Handle enriches the log record with [slog.Attr] stored in context with [WithAttrs].
func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if attrs, ok := ctx.Value(slogAttrs).([]slog.Attr); ok {
		for _, v := range attrs {
			r.AddAttrs(v)
		}
	}

	if err := h.handler.Handle(ctx, r); err != nil {
		return fmt.Errorf("handle log record: %w", err)
	}
	return nil
}

// WithAttrs returns a new ContextHandler that wraps the result of calling WithAttrs on the underlying handler.
func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ContextHandler{handler: h.handler.WithAttrs(attrs)}
}

// WithGroup returns a new ContextHandler that wraps the result of calling WithGroup on the underlying handler.
func (h *ContextHandler) WithGroup(name string) slog.Handler {
	return &ContextHandler{handler: h.handler.WithGroup(name)}
}

// WithAttrs adds [...slog.Attr] to the [context.Context] that enriches the log messages handled by [ContextHandler].
func WithAttrs(ctx context.Context, attr ...slog.Attr) context.Context {
	if v, ok := ctx.Value(slogAttrs).([]slog.Attr); ok {
		v = append(v, attr...)
		return context.WithValue(ctx, slogAttrs, v)
	}
	return context.WithValue(ctx, slogAttrs, attr)
}
