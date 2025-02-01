package pprofserver

import (
	"context"
	"github.com/myrjola/petrapp/internal/errors"
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
		return errors.Wrap(err, "pprof listen and serve")
	}
	return nil
}

func Launch(ctx context.Context, addr string, logger *slog.Logger) {
	go func() {
		logger.LogAttrs(ctx, slog.LevelInfo, "starting pprof server", slog.String("addr", addr))
		if err := listenAndServe(addr); err != nil {
			logger.LogAttrs(ctx, slog.LevelError, "failed starting pprof server", errors.SlogError(err))
		}
	}()
}
