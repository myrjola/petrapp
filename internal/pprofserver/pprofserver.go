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

func listenAndServe(addr string) error {
	err := newServer(addr).ListenAndServe()
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
		if err := listenAndServe(addr); err != nil {
			logger.LogAttrs(ctx, slog.LevelError, "failed starting pprof server", slog.Any("error", err))
		}
	}()
}
