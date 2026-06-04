package main

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
)

type renderer struct {
	tmpl *template.Template
}

func newRenderer() (*renderer, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getwd: %w", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, errors.New("go.mod not found from working dir")
		}
		dir = parent
	}
	root := filepath.Join(dir, "cmd", "example", "ui", "templates")
	tmpl, err := template.ParseGlob(filepath.Join(root, "*.gohtml"))
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	if _, err = tmpl.ParseGlob(filepath.Join(root, "pages", "*.gohtml")); err != nil {
		return nil, fmt.Errorf("parse page templates: %w", err)
	}
	return &renderer{tmpl: tmpl}, nil
}

// render writes a parsed template to the response. It is consumed by the todo
// CRUD and /account handlers.
func (rnd *renderer) render(w http.ResponseWriter, status int, name string, data any) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := rnd.tmpl.ExecuteTemplate(w, name, data); err != nil {
		return fmt.Errorf("execute template %q: %w", name, err)
	}
	return nil
}

func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "deny")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func (app *application) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				app.logger.ErrorContext(r.Context(), "panic recovered", "err", rec)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (app *application) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthy", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /", app.handleList)
	mux.HandleFunc("GET /todos/{id}", app.handleDetail)
	mux.HandleFunc("POST /todos", app.handleCreate)
	mux.HandleFunc("POST /todos/{id}/toggle", app.handleToggle)
	mux.HandleFunc("POST /todos/{id}/delete", app.handleDelete)

	// Shared passkey auth wrappers (thin handlers around the auth handler).
	mux.HandleFunc("POST /api/registration/begin", app.beginRegistration)
	mux.HandleFunc("POST /api/registration/finish", app.finishRegistration)
	mux.HandleFunc("POST /api/login/begin", app.beginLogin)
	mux.HandleFunc("POST /api/login/finish", app.finishLogin)
	mux.HandleFunc("POST /api/logout", app.logout)
	// Demonstrate the shared gate without forcing a passkey ceremony on the
	// CRUD pages (kept open so handler tests need no auth). /account is gated.
	mux.Handle("GET /account", app.auth.AuthenticateMiddleware(
		http.HandlerFunc(app.handleAccount)))

	var handler http.Handler = mux
	handler = app.recoverPanic(handler)
	handler = secureHeaders(handler)
	handler = app.sessionManager.LoadAndSave(handler)
	return handler
}
