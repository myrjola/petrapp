package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/myrjola/petrapp/internal/e2etest"
)

const defaultTimeout = 2 * time.Second

// createListener creates a TCP listener and returns it together with the logical address.
// The logical address uses the configured host (e.g. "localhost") and the actual bound port,
// so that WebAuthn RP origins and the e2etest server URL agree even when port 0 is used.
func createListener(ctx context.Context, addr string) (net.Listener, string, error) {
	idleTimeout := 2 * time.Minute //nolint:mnd // reverse proxy may keep connections open for a long time.
	listenCfg := net.ListenConfig{
		Control:   nil,
		KeepAlive: idleTimeout,
		KeepAliveConfig: net.KeepAliveConfig{
			Enable:   true,
			Idle:     idleTimeout,
			Interval: 0,
			Count:    0,
		},
	}
	listener, err := listenCfg.Listen(ctx, "tcp", addr)
	if err != nil {
		return nil, "", fmt.Errorf("TCP listen: %w", err)
	}
	// Preserve the configured host so that "localhost:0" → "localhost:PORT", not "127.0.0.1:PORT".
	configuredHost, _, _ := net.SplitHostPort(addr)
	_, actualPort, _ := net.SplitHostPort(listener.Addr().String())
	logicalAddr := net.JoinHostPort(configuredHost, actualPort)
	return listener, logicalAddr, nil
}

// configureAndStartServer configures and starts the HTTP server using an already-bound listener.
// logAddr is the human-readable address logged and used by e2etest to build the server URL.
func (app *application) configureAndStartServer(
	ctx context.Context, listener net.Listener, logAddr string, handler http.Handler,
) error {
	var err error
	shutdownComplete := make(chan struct{})
	idleTimeout := 2 * time.Minute //nolint:mnd // reverse proxy may keep connections open for a long time.
	srv := &http.Server{
		ErrorLog:          slog.NewLogLogger(app.logger.Handler(), slog.LevelError),
		Handler:           handler,
		IdleTimeout:       idleTimeout,
		ReadTimeout:       defaultTimeout,
		WriteTimeout:      defaultTimeout,
		ReadHeaderTimeout: time.Second,
		MaxHeaderBytes:    1 << 20, //nolint:mnd // 1 MB
	}

	// Create a shutdown goroutine that handles graceful shutdown
	go func() {
		defer close(shutdownComplete)

		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		signal.Notify(sigint, syscall.SIGTERM)

		var shutdownReason string
		select {
		case <-sigint:
			shutdownReason = "signal"
		case <-ctx.Done():
			shutdownReason = "context"
		}

		// Create a new context for logging since the original might be cancelled
		logCtx := context.WithoutCancel(ctx)
		now := time.Now()
		app.logger.LogAttrs(logCtx, slog.LevelInfo, "shutting down server", slog.String("reason", shutdownReason))

		// We received an interrupt signal or context cancellation, shut down.
		var shutdownContext context.Context
		var cancel context.CancelFunc
		shutdownContext, cancel = context.WithTimeout(context.WithoutCancel(ctx), defaultTimeout)
		defer cancel()
		if shutdownErr := srv.Shutdown(shutdownContext); shutdownErr != nil {
			shutdownErr = fmt.Errorf("shutdown server: %w", shutdownErr)
			app.logger.LogAttrs(logCtx, slog.LevelError, "error shutting down server", slog.Any("error", shutdownErr))
		}

		// Stop flight recorder after server shutdown
		if app.flightRecorder != nil {
			app.flightRecorder.Stop(logCtx)
		}

		app.logger.LogAttrs(logCtx, slog.LevelInfo, "server shut down", slog.Duration("duration", time.Since(now)))
	}()

	app.logger.LogAttrs(ctx, slog.LevelInfo, "starting server", slog.Any(e2etest.LogAddrKey, logAddr))
	if err = srv.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server serve: %w", err)
	}

	// Wait for the shutdown goroutine to complete all cleanup work
	<-shutdownComplete

	return nil
}
