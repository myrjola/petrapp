package main

import (
	"fmt"
	"net/http"
	"os"
	"path"
)

// notFoundInterceptor wraps http.ResponseWriter so we can detect when
// http.FileServer returns 404 (file not found) and substitute our custom
// 404 page instead. This eliminates the per-request os.Stat the handler
// previously used to make the same decision up-front.
//
// The interceptor buffers the WriteHeader call so headers set by upstream
// middleware (e.g. Cache-Control: public from cacheForever) are not flushed
// before we know the response is a 404.
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
		// be rendered as text/html.
		h := i.Header()
		h.Del("Content-Type")
		h.Del("X-Content-Type-Options")
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

// fileServerHandler creates a file server handler with custom 404 handling.
func (app *application) fileServerHandler() (http.Handler, error) {
	fileRoot := path.Join(".", "ui", "static")
	if _, err := os.Stat(fileRoot); os.IsNotExist(err) {
		dir, findErr := findModuleDir()
		if findErr != nil {
			return nil, fmt.Errorf("findModuleDir: %w", findErr)
		}
		fileRoot = path.Join(dir, "ui", "static")
	}
	stat, err := os.Stat(fileRoot)
	if err != nil || !stat.IsDir() {
		return nil, fmt.Errorf("file server root %s does not exist or is not a directory", fileRoot)
	}

	fileServer := http.FileServer(http.Dir(fileRoot))
	notFoundHandler := app.sessionDeltaStack(http.HandlerFunc(app.notFound))

	return app.noAuthStack(cacheForever(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			interceptor := &notFoundInterceptor{ResponseWriter: w, is404: false, headerWritten: false}
			fileServer.ServeHTTP(interceptor, r)
			if interceptor.is404 {
				notFoundHandler.ServeHTTP(w, r)
			}
		}))), nil
}
