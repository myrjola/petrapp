package testkit

import (
	"io"
	"log/slog"

	"github.com/myrjola/petrapp/internal/platform/obs/logging"
)

// NewLogger creates a new logger with the given log sink such as testkit.Writer.
func NewLogger(logSink io.Writer) *slog.Logger {
	handler := logging.NewContextHandler(slog.NewTextHandler(logSink, &slog.HandlerOptions{
		AddSource:   false,
		Level:       slog.LevelDebug,
		ReplaceAttr: nil,
	}))
	return slog.New(handler)
}
