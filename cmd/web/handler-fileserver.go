package main

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// fileServerHandler creates a file server handler with custom 404 handling.
func (app *application) fileServerHandler() (http.Handler, error) {
	fileRoot := path.Join(".", "ui", "static")
	var err error
	if _, err = os.Stat(fileRoot); os.IsNotExist(err) {
		var dir string
		dir, err = findModuleDir()
		if err != nil {
			return nil, fmt.Errorf("findModuleDir: %w", err)
		}
		fileRoot = path.Join(dir, "ui", "static")
	}
	var stat os.FileInfo
	if stat, err = os.Stat(fileRoot); os.IsNotExist(err) || !stat.IsDir() {
		return nil, fmt.Errorf("file server root %s does not exist or is not a directory", fileRoot)
	}
	httpDir := http.Dir(fileRoot)

	// File server with custom 404 handling
	fileServer := http.FileServer(httpDir)

	return app.noAuthStack(cacheForever(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if this is a request for a static file that doesn't exist
			// Sanitize the URL path to prevent directory traversal attacks
			cleanPath := filepath.Clean(r.URL.Path)
			if strings.Contains(cleanPath, "..") {
				// Path contains directory traversal, use 404 handler
				app.sessionDeltaStack(http.HandlerFunc(app.notFound)).ServeHTTP(w, r)
				return
			}
			staticPath := filepath.Join(fileRoot, cleanPath)
			if _, statErr := os.Stat(staticPath); os.IsNotExist(statErr) {
				// File doesn't exist, use our custom 404 handler with session middleware
				app.sessionDeltaStack(http.HandlerFunc(app.notFound)).ServeHTTP(w, r)
				return
			}

			// File exists, serve it normally
			fileServer.ServeHTTP(w, r)
		}))), nil
}
