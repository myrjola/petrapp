package pprofserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"time"
)

func Handle(mux *http.ServeMux) {
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
}

func newServeMux() *http.ServeMux {
	mux := http.NewServeMux()
	Handle(mux)
	return mux
}

func newServer(addr string) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           newServeMux(),
		ReadHeaderTimeout: 1 * time.Second,
	}
}

// shutdownTimeout bounds how long Shutdown waits for in-flight pprof requests
// to finish (profiles can take a while). Long enough for a 30s CPU profile to
// drain, short enough that process exit isn't blocked on a stuck client.
const shutdownTimeout = 35 * time.Second

func listenAndServe(ctx context.Context, addr string) error {
	srv := newServer(addr)
	go func() {
		<-ctx.Done()
		// Detach from ctx (which is already done) so Shutdown actually has
		// time to drain in-flight requests.
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	err := srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("pprof listen and serve: %w", err)
	}
	return nil
}

func Launch(ctx context.Context, addr string, logger *slog.Logger) {
	go func() {
		logger.LogAttrs(ctx, slog.LevelInfo, "starting pprof server", slog.String("addr", addr))
		if err := listenAndServe(ctx, addr); err != nil {
			logger.LogAttrs(ctx, slog.LevelError, "failed starting pprof server", slog.Any("error", err))
		}
	}()
}
