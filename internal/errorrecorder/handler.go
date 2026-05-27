package errorrecorder

import (
	"context"
	"fmt"
	"log/slog"
)

// Handler is the slog.Handler decorator that feeds records into a Service's
// buffers. It is constructed via Service.Handler(); callers do not
// instantiate it directly.
//
// withAttrs accumulates the attrs added via WithAttrs so that they remain
// visible to the per-session key resolver even though slog does not embed
// them in the record itself. The recorder ignores groups for keying — only
// top-level session_hash / trace_id attrs matter, so WithGroup does not need
// to alter withAttrs.
type Handler struct {
	service     *Service
	inner       slog.Handler
	withAttrs   []slog.Attr
	withinGroup bool
}

// Enabled defers to the inner handler.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle forwards rec to the inner handler first (stdout must not be affected
// by the recorder), then feeds the record into the per-session buffer.
// Attrs added via WithAttrs are re-attached to the cloned record passed to
// observe so the session-key resolver can see them.
func (h *Handler) Handle(ctx context.Context, rec slog.Record) error {
	if err := h.inner.Handle(ctx, rec); err != nil {
		return fmt.Errorf("inner handle: %w", err)
	}
	if len(h.withAttrs) > 0 {
		cloned := rec.Clone()
		cloned.AddAttrs(h.withAttrs...)
		h.service.observe(cloned)
		return nil
	}
	h.service.observe(rec)
	return nil
}

// WithAttrs returns a Handler whose inner is the inner handler's
// WithAttrs result. The Service pointer is preserved so all clones share
// the same buffer map. Attrs added while inside a group are not tracked
// for key resolution because the recorder only looks at top-level attrs.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := &Handler{
		service:     h.service,
		inner:       h.inner.WithAttrs(attrs),
		withAttrs:   h.withAttrs,
		withinGroup: h.withinGroup,
	}
	if !h.withinGroup {
		clone.withAttrs = append(append([]slog.Attr{}, h.withAttrs...), attrs...)
	}
	return clone
}

// WithGroup returns a Handler whose inner is the inner handler's
// WithGroup result. The Service pointer is preserved. Once inside a group,
// subsequent WithAttrs calls do not contribute top-level keying attrs.
func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{
		service:     h.service,
		inner:       h.inner.WithGroup(name),
		withAttrs:   h.withAttrs,
		withinGroup: true,
	}
}
