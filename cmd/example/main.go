package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alexedwards/scs/sqlite3store"
	"github.com/alexedwards/scs/v2"
	"github.com/myrjola/petrapp/cmd/example/internal/todo"
	"github.com/myrjola/petrapp/internal/platform/auth"
	"github.com/myrjola/petrapp/internal/platform/envstruct"
	"github.com/myrjola/petrapp/internal/platform/obs/logging"
	"github.com/myrjola/petrapp/internal/platform/sqlitekit"
)

// readHeaderTimeout bounds how long the server waits for request headers,
// guarding against slow-loris clients.
const readHeaderTimeout = 5 * time.Second

type config struct {
	Addr      string `env:"EXAMPLE_ADDR"       envDefault:"localhost:8082"`
	FQDN      string `env:"EXAMPLE_FQDN"       envDefault:"localhost"`
	SqliteURL string `env:"EXAMPLE_SQLITE_URL" envDefault:":memory:"`
}

type application struct {
	logger         *slog.Logger
	repo           *todo.Repository
	auth           *auth.WebAuthnHandler
	sessionManager *scs.SessionManager
	renderer       *renderer
}

func main() {
	logger := slog.New(logging.NewContextHandler(
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{ //nolint:exhaustruct // only Level is set.
			Level: slog.LevelInfo,
		})))
	if err := run(logger); err != nil {
		logger.Error("startup failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	var cfg config
	if err := envstruct.Populate(&cfg, os.LookupEnv); err != nil {
		return fmt.Errorf("populate config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, cleanup, err := newApplication(ctx, logger, cfg)
	if err != nil {
		return fmt.Errorf("new application: %w", err)
	}
	defer cleanup()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.routes(),
		ReadHeaderTimeout: readHeaderTimeout,
	}
	logger.Info("listening", slog.String("addr", cfg.Addr))
	if err = srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("listen and serve: %w", err)
	}
	return nil
}

func newApplication(
	ctx context.Context, logger *slog.Logger, cfg config,
) (*application, func(), error) {
	db, err := sqlitekit.NewDatabase(ctx, sqlitekit.Config{
		URL:      cfg.SqliteURL,
		Schema:   auth.SchemaSQL + "\n" + schemaSQL,
		Fixtures: "",
		Logger:   logger,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("new database: %w", err)
	}

	sessionManager := scs.New()
	sessionManager.Store = sqlite3store.New(db.ReadWrite)
	sessionManager.Lifetime = 24 * time.Hour

	authHandler, err := auth.New(
		cfg.Addr, cfg.FQDN, false, logger, sessionManager, auth.NewSQLiteStore(db))
	if err != nil {
		return nil, nil, fmt.Errorf("new auth handler: %w", err)
	}

	rnd, err := newRenderer()
	if err != nil {
		return nil, nil, fmt.Errorf("new renderer: %w", err)
	}

	app := &application{
		logger:         logger,
		repo:           todo.NewRepository(db),
		auth:           authHandler,
		sessionManager: sessionManager,
		renderer:       rnd,
	}
	return app, func() { _ = db.Close() }, nil
}

// newTestApplication is the test seam used by the package tests.
func newTestApplication(
	t interface{ Context() context.Context }, logger *slog.Logger,
) (*application, func(), error) {
	return newApplication(t.Context(), logger, config{
		Addr:      "localhost:0",
		FQDN:      "localhost",
		SqliteURL: ":memory:",
	})
}
