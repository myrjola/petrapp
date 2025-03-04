package main

import (
	"context"
	"fmt"
	"github.com/alexedwards/scs/sqlite3store"
	"github.com/alexedwards/scs/v2"
	"github.com/myrjola/petrapp/internal/envstruct"
	"github.com/myrjola/petrapp/internal/logging"
	"github.com/myrjola/petrapp/internal/pprofserver"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/webauthnhandler"
	"github.com/myrjola/petrapp/internal/workout"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"
)

type application struct {
	logger          *slog.Logger
	webAuthnHandler *webauthnhandler.WebAuthnHandler
	sessionManager  *scs.SessionManager
	templateFS      fs.FS
	workoutService  *workout.Service
}

type config struct {
	// Addr is the address to listen on. It's possible to choose the address dynamically with localhost:0.
	Addr string `env:"PETRAPP_ADDR" envDefault:"localhost:8081"`
	// FQDN is the fully qualified domain name of the server used for WebAuthn Relying Party configuration.
	FQDN string `env:"PETRAPP_FQDN" envDefault:"localhost"`
	// FlyAppName is the name of the Fly application. It's used to override the FQDN.
	FlyAppName string `env:"FLY_APP_NAME" envDefault:""`
	// SqliteURL is the URL to the SQLite database. You can use ":memory:" for an ethereal in-memory database.
	SqliteURL string `env:"PETRAPP_SQLITE_URL" envDefault:"./petrapp.sqlite3"`
	// PProfAddr is the optional address to listen on for the pprof server.
	PProfAddr string `env:"PETRAPP_PPROF_ADDR" envDefault:""`
	// TemplatePath is the path to the directory containing the HTML templates.
	TemplatePath string `env:"PETRAPP_TEMPLATE_PATH" envDefault:""`
}

func run(ctx context.Context, logger *slog.Logger, lookupEnv func(string) (string, bool)) error {
	var (
		cancel context.CancelFunc
		err    error
	)

	ctx, cancel = signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	var cfg config
	if err = envstruct.Populate(&cfg, lookupEnv); err != nil {
		return fmt.Errorf("populate config: %w", err)
	}

	if cfg.PProfAddr != "" {
		pprofserver.Launch(ctx, cfg.PProfAddr, logger)
	}

	var htmlTemplatePath string
	if htmlTemplatePath, err = resolveAndVerifyTemplatePath(cfg.TemplatePath); err != nil {
		return fmt.Errorf("resolve template path: %w", err)
	}

	db, err := sqlite.NewDatabase(ctx, cfg.SqliteURL, logger)
	if err != nil {
		return fmt.Errorf("open db (url: %s): %w", cfg.SqliteURL, err)
	}
	logger.LogAttrs(ctx, slog.LevelInfo, "connected to db")

	sessionManager := initializeSessionManager(db)

	fqdn := cfg.FQDN
	if cfg.FlyAppName != "" {
		fqdn = cfg.FlyAppName + ".fly.dev"
	}
	var webAuthnHandler *webauthnhandler.WebAuthnHandler
	if webAuthnHandler, err = webauthnhandler.New(cfg.Addr, fqdn, logger, sessionManager, db); err != nil {
		return fmt.Errorf("new webauthn handler: %w", err)
	}

	app := application{
		logger:          logger,
		webAuthnHandler: webAuthnHandler,
		sessionManager:  sessionManager,
		templateFS:      os.DirFS(htmlTemplatePath),
		workoutService:  workout.NewService(db, logger),
	}

	if err = app.configureAndStartServer(ctx, cfg.Addr); err != nil {
		return fmt.Errorf("start server: %w", err)
	}
	return nil
}

func initializeSessionManager(dbs *sqlite.Database) *scs.SessionManager {
	sessionManager := scs.New()
	sessionManager.Store = sqlite3store.NewWithCleanupInterval(dbs.ReadWrite, 24*time.Hour) //nolint:mnd // day
	sessionManager.Lifetime = 12 * time.Hour                                                //nolint:mnd // half a day
	sessionManager.Cookie.Persist = true
	sessionManager.Cookie.Secure = true
	sessionManager.Cookie.HttpOnly = true
	sessionManager.Cookie.SameSite = http.SameSiteStrictMode
	return sessionManager
}

func main() {
	ctx := context.Background()
	loggerHandler := logging.NewContextHandler(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource:   false,
		Level:       slog.LevelDebug,
		ReplaceAttr: nil,
	}))
	logger := slog.New(loggerHandler)
	if err := run(ctx, logger, os.LookupEnv); err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "failure starting application", slog.Any("error", err))
		os.Exit(1)
	}
}
