package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/alexedwards/scs/sqlite3store"
	"github.com/alexedwards/scs/v2"
	"github.com/myrjola/petrapp/internal/envstruct"
	"github.com/myrjola/petrapp/internal/flightrecorder"
	"github.com/myrjola/petrapp/internal/logging"
	"github.com/myrjola/petrapp/internal/notification"
	"github.com/myrjola/petrapp/internal/pprofserver"
	"github.com/myrjola/petrapp/internal/service"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/webauthnhandler"
)

type application struct {
	logger          *slog.Logger
	webAuthnHandler *webauthnhandler.WebAuthnHandler
	sessionManager  *scs.SessionManager
	templateFS      fs.FS
	// parsedTemplates memoizes page templates so renders skip filesystem reads
	// and re-parsing. Bypassed in devMode so template edits surface on refresh.
	parsedTemplates *templateCache
	service         *service.Service
	flightRecorder  *flightrecorder.Service
	// devMode is true when running outside the Fly.io production deployment.
	// It enables developer-only routes like /dev/styleguide.
	devMode bool
	// vapidPublicKey is the VAPID public key (base64url, uncompressed P-256)
	// exposed to the browser so it can call pushManager.subscribe. Wired in
	// from config / env in a later task; empty here means push subscribes will
	// fail with a runtime error surfaced to the user, while the UI still
	// renders correctly.
	vapidPublicKey string
	// lastRequestAt is updated by stampLastRequest middleware on every
	// request. notification.IdleMonitor reads it to gate process exit so the
	// Fly Machine can scale to zero between workouts.
	lastRequestAt *atomic.Int64
}

type config struct {
	// Addr is the address to listen on. It's possible to choose the address dynamically with localhost:0.
	Addr string `env:"PETRAPP_ADDR" envDefault:"localhost:8081"`
	// FQDN is the fully qualified domain name of the server used for WebAuthn Relying Party configuration.
	FQDN string `env:"PETRAPP_FQDN" envDefault:"localhost"`
	// FlyAppName is the name of the Fly application. It's used to override the FQDN.
	FlyAppName string `env:"FLY_APP_NAME" envDefault:""`
	// TLSCert is the path to the TLS certificate file. When both TLSCert and TLSKey are set, the
	// server starts with TLS using ServeTLS instead of Serve.
	TLSCert string `env:"PETRAPP_TLS_CERT" envDefault:""`
	// TLSKey is the path to the TLS key file. See TLSCert.
	TLSKey string `env:"PETRAPP_TLS_KEY" envDefault:""`
	// SqliteURL is the URL to the SQLite database. You can use ":memory:" for an ethereal in-memory database.
	SqliteURL string `env:"PETRAPP_SQLITE_URL" envDefault:"./petrapp.sqlite3"`
	// PProfAddr is the optional address to listen on for the pprof server.
	PProfAddr string `env:"PETRAPP_PPROF_ADDR" envDefault:""`
	// TemplatePath is the path to the directory containing the HTML templates.
	TemplatePath string `env:"PETRAPP_TEMPLATE_PATH" envDefault:""`
	// TracesDirectory is the path to the directory where trace files are written.
	TracesDirectory string `env:"PETRAPP_TRACES_DIRECTORY" envDefault:""`
	// OpenAIAPIKey is optional. It's used to authenticate with the OpenAI API.
	OpenAIAPIKey string `env:"OPENAI_API_KEY" envDefault:""`
	// VAPIDPublic is the base64url-encoded VAPID public key used by both the
	// server (to sign push JWTs) and the client (passed as applicationServerKey
	// to pushManager.subscribe). Generated ephemerally in dev when empty.
	VAPIDPublic string `env:"PETRAPP_VAPID_PUBLIC" envDefault:""`
	// VAPIDPrivate is the base64url-encoded VAPID private key. Must be set in
	// production via Fly secrets; dev generates an ephemeral pair when empty.
	VAPIDPrivate string `env:"PETRAPP_VAPID_PRIVATE" envDefault:""`
	// VAPIDSubject is the email passed as the JWT sub claim. Bare email — the
	// webpush-go library prepends "mailto:" itself.
	VAPIDSubject string `env:"PETRAPP_VAPID_SUBJECT" envDefault:"vapid@example.com"`
	// NotificationIdleTimeoutSec is the idle-monitor threshold in seconds.
	// Stored as a string env var because envstruct only handles strings;
	// parsed inside run().
	NotificationIdleTimeoutSec string `env:"PETRAPP_NOTIFICATION_IDLE_TIMEOUT_SECONDS" envDefault:"300"`
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
	defer func() {
		if err = db.Close(); err != nil {
			logger.LogAttrs(ctx, slog.LevelError, "failed to close database", slog.Any("error", err))
		}
	}()
	logger.LogAttrs(ctx, slog.LevelInfo, "connected to db")

	sessionManager := initializeSessionManager(db)

	// Bind the listener first so we know the actual port before configuring WebAuthn.
	// This matters when port 0 is used (e.g. in tests): the RP origin must match the URL
	// the browser navigates to, and both are derived from the logical address below.
	var listener net.Listener
	var actualAddr string
	if listener, actualAddr, err = createListener(ctx, cfg.Addr); err != nil {
		return fmt.Errorf("create listener: %w", err)
	}

	fqdn := cfg.FQDN
	if cfg.FlyAppName != "" {
		fqdn = cfg.FlyAppName + ".fly.dev"
	}
	var webAuthnHandler *webauthnhandler.WebAuthnHandler
	tlsEnabled := cfg.TLSCert != ""
	if webAuthnHandler, err = webauthnhandler.New(
		actualAddr,
		fqdn,
		tlsEnabled,
		logger,
		sessionManager,
		db,
	); err != nil {
		return fmt.Errorf("new webauthn handler: %w", err)
	}

	var flightRecorderService *flightrecorder.Service
	if cfg.TracesDirectory != "" {
		if flightRecorderService, err = flightrecorder.New(flightrecorder.Config{
			Logger:          logger,
			MinAge:          0, // Use default
			MaxBytes:        0, // Use default
			TracesDirectory: cfg.TracesDirectory,
		}); err != nil {
			return fmt.Errorf("new flight recorder: %w", err)
		}
		// Start flight recording.
		if err = flightRecorderService.Start(ctx); err != nil {
			return fmt.Errorf("start flight recorder: %w", err)
		}
	}

	if err = ensureVAPIDKeys(ctx, &cfg, logger); err != nil {
		return err
	}

	notif, err := buildNotificationStack(ctx, &cfg, db, logger)
	if err != nil {
		return err
	}
	go notif.idleMonitor.Run(ctx)

	app := newApplication(
		logger,
		webAuthnHandler,
		sessionManager,
		htmlTemplatePath,
		notif.svc,
		flightRecorderService,
		cfg.FlyAppName == "",
		cfg.VAPIDPublic,
		notif.lastRequestAt,
	)

	routes, err := app.routes()
	if err != nil {
		return fmt.Errorf("initialize routes: %w", err)
	}

	return app.configureAndStartServer(ctx, listener, actualAddr, cfg.TLSCert, cfg.TLSKey, routes)
}

// newApplication builds the *application and wires webauthnhandler's
// InternalErrorHandler to the shim-aware serverError so DB / lookup failures
// inside that middleware navigate to /error instead of producing a silent 500.
func newApplication(
	logger *slog.Logger,
	webAuthnHandler *webauthnhandler.WebAuthnHandler,
	sessionManager *scs.SessionManager,
	htmlTemplatePath string,
	svc *service.Service,
	flightRecorderService *flightrecorder.Service,
	devMode bool,
	vapidPublicKey string,
	lastRequestAt *atomic.Int64,
) *application {
	app := &application{
		logger:          logger,
		webAuthnHandler: webAuthnHandler,
		sessionManager:  sessionManager,
		templateFS:      os.DirFS(htmlTemplatePath),
		parsedTemplates: newTemplateCache(),
		service:         svc,
		flightRecorder:  flightRecorderService,
		devMode:         devMode,
		vapidPublicKey:  vapidPublicKey,
		lastRequestAt:   lastRequestAt,
	}
	webAuthnHandler.InternalErrorHandler = app.serverError
	return app
}

const (
	// sessionStoreCleanupInterval bounds how often expired sessions are
	// pruned from sqlite3store. Daily is plenty given Lifetime below.
	sessionStoreCleanupInterval = 24 * time.Hour

	// sessionLifetime keeps users logged in across mid-workout sessions
	// so a 7am passkey login doesn't expire before the evening's lift.
	sessionLifetime = 7 * 24 * time.Hour

	// maintenanceCacheTTL bounds how stale the cached maintenance_mode flag
	// may be before the next request re-reads it from the database. Toggling
	// maintenance is an interactive admin action, so a few seconds of lag is
	// well below human reaction time; Service.SetFeatureFlag invalidates the
	// cache anyway so the TTL only matters for out-of-band writes.
	maintenanceCacheTTL = 5 * time.Second
)

// ensureVAPIDKeys validates the VAPID config: petra and petra-staging require
// both keys to be set via Fly secrets; dev and pr-* review apps generate an
// ephemeral pair on each start and log the public key. Review-app push delivery
// won't actually reach real browsers (the public key rotates per boot), but the
// app boots and the UI renders.
func ensureVAPIDKeys(ctx context.Context, cfg *config, logger *slog.Logger) error {
	if cfg.VAPIDPublic != "" && cfg.VAPIDPrivate != "" {
		return nil
	}
	if cfg.FlyAppName != "" && !strings.HasPrefix(cfg.FlyAppName, "pr-") {
		return errors.New("PETRAPP_VAPID_PUBLIC and PETRAPP_VAPID_PRIVATE must be set in production")
	}
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return fmt.Errorf("generate dev vapid keys: %w", err)
	}
	cfg.VAPIDPrivate, cfg.VAPIDPublic = priv, pub
	logger.LogAttrs(ctx, slog.LevelWarn, "generated ephemeral VAPID keys",
		slog.String("public", pub))
	return nil
}

// notificationStack groups the wired notification pieces handed back to run().
type notificationStack struct {
	svc           *service.Service
	idleMonitor   *notification.IdleMonitor
	lastRequestAt *atomic.Int64
}

// buildNotificationStack wires Sender + Scheduler + IdleMonitor and returns the
// Scheduler-aware Service plus the lastRequestAt atomic the stamping middleware
// updates.
func buildNotificationStack(
	ctx context.Context,
	cfg *config,
	db *sqlite.Database,
	logger *slog.Logger,
) (*notificationStack, error) {
	idleSeconds, err := strconv.Atoi(cfg.NotificationIdleTimeoutSec)
	if err != nil {
		return nil, fmt.Errorf("parse PETRAPP_NOTIFICATION_IDLE_TIMEOUT_SECONDS: %w", err)
	}
	if idleSeconds <= 0 {
		return nil, fmt.Errorf("PETRAPP_NOTIFICATION_IDLE_TIMEOUT_SECONDS must be positive: got %d", idleSeconds)
	}
	idleTimeout := time.Duration(idleSeconds) * time.Second

	// HTTPClient is intentionally left unset so the Sender uses http.DefaultClient.
	senderCfg := notification.SenderConfig{ //nolint:exhaustruct // HTTPClient defaults to http.DefaultClient.
		VAPIDSubject:    cfg.VAPIDSubject,
		VAPIDPublicKey:  cfg.VAPIDPublic,
		VAPIDPrivateKey: cfg.VAPIDPrivate,
		Logger:          logger,
	}
	sender := notification.NewSender(senderCfg)

	baseService := service.NewService(db, logger, cfg.OpenAIAPIKey)

	scheduler := notification.NewScheduler(notification.SchedulerConfig{
		Repo:     baseService.Repos().ScheduledPushes,
		Dispatch: makeDispatchFunc(logger, baseService, sender),
		Logger:   logger,
		Now:      time.Now,
	})
	if err = scheduler.Reload(ctx); err != nil {
		return nil, fmt.Errorf("reload scheduled pushes: %w", err)
	}

	// Memoise the maintenance_mode feature flag in process when running on
	// Fly. Every HTTP request consults it via middleware; before caching,
	// that was one DB read per request. Service.SetFeatureFlag invalidates
	// the cache, so an admin toggle propagates immediately under normal
	// operation. Locally and in tests we leave caching off so raw-SQL flag
	// writes are observed immediately.
	svc := baseService.WithScheduler(scheduler)
	if cfg.FlyAppName != "" {
		svc = svc.WithMaintenanceCacheTTL(maintenanceCacheTTL)
	}

	lastRequestAt := new(atomic.Int64)
	lastRequestAt.Store(time.Now().UnixNano())

	const idleTickInterval = 10 * time.Second
	idleMonitor := notification.NewIdleMonitor(notification.IdleMonitorConfig{
		IdleThreshold: idleTimeout,
		TickInterval:  idleTickInterval,
		Now:           time.Now,
		LastRequestAt: func() time.Time { return time.Unix(0, lastRequestAt.Load()) },
		PendingCount:  scheduler.PendingCount,
		Trigger: func() {
			if killErr := syscall.Kill(os.Getpid(), syscall.SIGTERM); killErr != nil {
				logger.LogAttrs(ctx, slog.LevelError, "idle monitor SIGTERM failed",
					slog.Any("error", killErr))
			}
		},
		Logger: logger,
	})

	return &notificationStack{
		svc:           svc,
		idleMonitor:   idleMonitor,
		lastRequestAt: lastRequestAt,
	}, nil
}

func initializeSessionManager(dbs *sqlite.Database) *scs.SessionManager {
	sessionManager := scs.New()
	sessionManager.Store = sqlite3store.NewWithCleanupInterval(dbs.ReadWrite, sessionStoreCleanupInterval)
	sessionManager.Lifetime = sessionLifetime
	sessionManager.Cookie.Persist = true
	sessionManager.Cookie.Secure = true
	sessionManager.Cookie.HttpOnly = true
	sessionManager.Cookie.SameSite = http.SameSiteLaxMode
	return sessionManager
}

func main() {
	ctx := context.Background()
	handlerOptions := &slog.HandlerOptions{
		AddSource:   false,
		Level:       slog.LevelDebug,
		ReplaceAttr: nil,
	}
	var loggerHandler slog.Handler
	loggerHandler = slog.NewTextHandler(os.Stdout, handlerOptions)
	if os.Getenv("FLY_MACHINE_ID") != "" {
		loggerHandler = slog.NewJSONHandler(os.Stdout, handlerOptions)
	}
	loggerHandler = logging.NewContextHandler(loggerHandler)
	appName := os.Getenv("FLY_APP_NAME")
	if appName == "" {
		appName = "petra-local"
	}
	logger := slog.New(loggerHandler).With(slog.String("service_name", appName))
	if err := run(ctx, logger, os.LookupEnv); err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "failure starting application", slog.Any("error", err))
		os.Exit(1)
	}
}
