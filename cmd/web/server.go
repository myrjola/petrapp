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

// configureAndStartServer configures and starts the HTTP server.
func (app *application) configureAndStartServer(ctx context.Context, addr string, handler http.Handler) error {
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
	go func() {
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
		logCtx := context.Background()
		app.logger.LogAttrs(logCtx, slog.LevelInfo, "shutting down server", slog.String("reason", shutdownReason))

		// We received an interrupt signal or context cancellation, shut down.
		var shutdownContext context.Context
		var cancel context.CancelFunc
		shutdownContext, cancel = context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()
		if shutdownErr := srv.Shutdown(shutdownContext); shutdownErr != nil {
			shutdownErr = fmt.Errorf("shutdown server: %w", shutdownErr)
			app.logger.LogAttrs(logCtx, slog.LevelError, "error shutting down server", slog.Any("error", shutdownErr))
		}

		// Stop flight recorder after server shutdown
		app.flightRecorder.Stop(logCtx)

		close(shutdownComplete)
	}()

	var listener net.Listener
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
	if listener, err = listenCfg.Listen(ctx, "tcp", addr); err != nil {
		return fmt.Errorf("TCP listen: %w", err)
	}
	app.logger.LogAttrs(ctx, slog.LevelInfo, "starting server", slog.Any(e2etest.LogAddrKey, listener.Addr().String()))
	if err = srv.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server serve: %w", err)
	}
	<-shutdownComplete

	return nil
}
