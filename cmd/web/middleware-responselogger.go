package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type statusResponseWriter struct {
	http.ResponseWriter
	statusCode    int
	headerWritten bool
}

func newstatusResponseWriter(w http.ResponseWriter) *statusResponseWriter {
	return &statusResponseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		headerWritten:  false,
	}
}

func (mw *statusResponseWriter) WriteHeader(statusCode int) {
	mw.ResponseWriter.WriteHeader(statusCode)

	if !mw.headerWritten {
		mw.statusCode = statusCode
		mw.headerWritten = true
	}
}

func (mw *statusResponseWriter) Write(b []byte) (int, error) {
	mw.headerWritten = true
	written, err := mw.ResponseWriter.Write(b)
	if err != nil {
		return written, fmt.Errorf("write response: %w", err)
	}
	return written, nil
}

func (mw *statusResponseWriter) Unwrap() http.ResponseWriter {
	return mw.ResponseWriter
}

// logResponse is a middleware that logs the status code and duration of the response.
func (app *application) logResponse(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := newstatusResponseWriter(w)
		next.ServeHTTP(sw, r)
		level := slog.LevelInfo
		if sw.statusCode >= http.StatusInternalServerError {
			level = slog.LevelError
		}
		app.logger.LogAttrs(r.Context(), level, "request completed",
			slog.Any("status_code", sw.statusCode), slog.Duration("duration", time.Since(start)))
	})
}
