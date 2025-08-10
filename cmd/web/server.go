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
	idleTimeout := time.Minute
	srv := &http.Server{
		ErrorLog:          slog.NewLogLogger(app.logger.Handler(), slog.LevelError),
		Handler:           handler,
		IdleTimeout:       idleTimeout,
		ReadTimeout:       defaultTimeout,
		WriteTimeout:      defaultTimeout,
		ReadHeaderTimeout: time.Second,
	}
	go func() {
		sigint := make(chan os.Signal, 1)

		signal.Notify(sigint, os.Interrupt)
		signal.Notify(sigint, syscall.SIGTERM)

		<-sigint
		app.logger.LogAttrs(ctx, slog.LevelInfo, "shutting down server")

		// We received an interrupt signal, shut down.
		var shutdownContext context.Context
		var cancel context.CancelFunc
		shutdownContext, cancel = context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()
		if err = srv.Shutdown(shutdownContext); err != nil {
			err = fmt.Errorf("shutdown server: %w", err)
			app.logger.LogAttrs(ctx, slog.LevelError, "error shutting down server", slog.Any("error", err))
		}
		close(shutdownComplete)
	}()

	var listener net.Listener
	if listener, err = net.Listen("tcp", addr); err != nil {
		return fmt.Errorf("TCP listen: %w", err)
	}
	app.logger.LogAttrs(ctx, slog.LevelInfo, "starting server", slog.Any(e2etest.LogAddrKey, listener.Addr().String()))
	if err = srv.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server serve: %w", err)
	}
	<-shutdownComplete

	return nil
}
