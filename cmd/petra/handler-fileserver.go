package main

import (
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
)

// notFoundInterceptor wraps http.ResponseWriter so we can detect when
// http.FileServer returns 404 (file not found) and substitute our custom
// 404 page instead. This eliminates the per-request os.Stat the handler
// previously used to make the same decision up-front.
//
// The interceptor buffers the WriteHeader call so the static Cache-Control set
// by the handler before serving is not flushed before we know the response is a
// 404 (at which point it is removed; see WriteHeader).
type notFoundInterceptor struct {
	http.ResponseWriter

	is404         bool
	headerWritten bool
}

func (i *notFoundInterceptor) WriteHeader(status int) {
	if status == http.StatusNotFound {
		i.is404 = true
		// http.FileServer's 404 path goes through http.Error, which sets
		// Content-Type: text/plain and X-Content-Type-Options: nosniff
		// before WriteHeader. Remove them so the custom 404 template can
		// be rendered as text/html. Also drop the static Cache-Control set
		// for the (missing) asset so the 404 page isn't cached as immutable.
		h := i.Header()
		h.Del("Content-Type")
		h.Del("X-Content-Type-Options")
		h.Del("Cache-Control")
		return
	}
	i.headerWritten = true
	i.ResponseWriter.WriteHeader(status)
}

func (i *notFoundInterceptor) Write(b []byte) (int, error) {
	if i.is404 {
		// Discard the body http.FileServer would write ("404 page not found\n").
		return len(b), nil
	}
	if !i.headerWritten {
		i.headerWritten = true
		i.ResponseWriter.WriteHeader(http.StatusOK)
	}
	written, err := i.ResponseWriter.Write(b)
	if err != nil {
		return written, fmt.Errorf("write static asset: %w", err)
	}
	return written, nil
}

// fileServerHandler creates a file server handler with content-hashed asset
// resolution and custom 404 handling. It serves ui/static from app.staticFS
// (the embedded tree in prod, os.DirFS in dev). A fingerprinted request path
// like /main.<hash>.css is stripped back to the real file before serving;
// assets whose body links other assets (manifest.json) are served from the
// rewritten in-memory copy.
func (app *application) fileServerHandler() (http.Handler, error) {
	staticFS := app.staticFS
	if staticFS == nil {
		// Tests build *application literals without wiring staticFS; fall back
		// to the embedded tree (always compiled in) so routes() still builds.
		var err error
		if staticFS, err = fs.Sub(embeddedUI, "ui/static"); err != nil {
			return nil, fmt.Errorf("sub embedded static: %w", err)
		}
	}

	fileServer := http.FileServerFS(staticFS)
	notFoundHandler := app.sessionDeltaStack(http.HandlerFunc(app.notFound))

	return app.noAuthStack(app.staticAssetHandler(fileServer, notFoundHandler)), nil
}

// staticAssetHandler resolves a (possibly fingerprinted) request path to a real
// asset, sets the appropriate Cache-Control, and serves it — from the rewritten
// in-memory copy for processed assets (manifest.json), otherwise from fileServer
// with the hash stripped off the path. Genuine misses fall through to
// notFoundHandler. Split out from fileServerHandler so it is unit-testable
// without the session-bearing middleware stacks.
func (app *application) staticAssetHandler(fileServer, notFoundHandler http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		realPath, exact := app.assets.resolve(r.URL.Path)
		app.setStaticCacheControl(w, exact)

		// Serve assets with rewritten bodies (hashed icon srcs) from memory.
		if pa, ok := app.assets.processedAssetFor(realPath); ok {
			w.Header().Set("Content-Type", pa.contentType)
			if _, err := w.Write(pa.body); err != nil {
				app.logger.LogAttrs(r.Context(), slog.LevelError,
					"write processed asset", slog.Any("error", err))
			}
			return
		}

		interceptor := &notFoundInterceptor{ResponseWriter: w, is404: false, headerWritten: false}
		served := r
		if realPath != r.URL.Path {
			served = r.Clone(r.Context())
			served.URL.Path = realPath
		}
		fileServer.ServeHTTP(interceptor, served)
		if interceptor.is404 {
			notFoundHandler.ServeHTTP(w, r)
		}
	}
}

// setStaticCacheControl sets the Cache-Control header for a static asset
// response. Dev always disables caching so edits surface on refresh. In prod a
// request that carried the current content hash is immutable; a plain or stale
// path is only revalidated so it can never go stale for a year.
func (app *application) setStaticCacheControl(w http.ResponseWriter, exact bool) {
	switch {
	case app.devMode:
		w.Header().Set("Cache-Control", "no-store, max-age=0, must-revalidate")
	case exact:
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	default:
		w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
	}
}
