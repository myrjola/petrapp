package testhelpers

import (
	"github.com/myrjola/petrapp/internal/logging"
	"io"
	"log/slog"
)

// NewLogger creates a new logger with the given log sink such as testhelpers.Writer.
func NewLogger(logSink io.Writer) *slog.Logger {
	handler := logging.NewContextHandler(slog.NewTextHandler(logSink, &slog.HandlerOptions{
		AddSource:   false,
		Level:       slog.LevelDebug,
		ReplaceAttr: nil,
	}))
	return slog.New(handler)
}
