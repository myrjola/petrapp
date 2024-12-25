package main

import (
	"context"
	"github.com/myrjola/sheerluck/internal/errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func (app *application) configureAndStartServer(ctx context.Context, addr string) error {
	var err error
	shutdownComplete := make(chan struct{})
	idleTimeout := time.Minute
	defaultTimeout := 5 * time.Second //nolint:mnd // 5 seconds should be enough even for slow LLM APIs.
	srv := &http.Server{
		ErrorLog:          slog.NewLogLogger(app.logger.Handler(), slog.LevelError),
		Handler:           timeoutHandler(app.routes(), defaultTimeout),
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
			err = errors.Wrap(err, "shutdown server")
			app.logger.LogAttrs(ctx, slog.LevelError, "error shutting down server", errors.SlogError(err))
		}
		close(shutdownComplete)
	}()

	var listener net.Listener
	if listener, err = net.Listen("tcp", addr); err != nil {
		return errors.Wrap(err, "TCP listen")
	}
	app.logger.LogAttrs(ctx, slog.LevelInfo, "starting server", slog.Any("Addr", listener.Addr().String()))
	if err = srv.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return errors.Wrap(err, "server serve")
	}
	<-shutdownComplete

	return nil
}
