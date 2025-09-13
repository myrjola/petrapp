package e2etest

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/myrjola/petrapp/internal/logging"
)

type Server struct {
	url        string
	client     *Client
	db         *sql.DB
	cancel     context.CancelCauseFunc
	serverDone chan struct{}
}

// LogAddrKey is the key used to log the address the server is listening on.
const LogAddrKey = "addr"

// LogDsnKey is the data source name key used to log the SQL DSN.
const LogDsnKey = "sqlDsn"

// StartServer starts the test server, waits for it to be ready, and return the server URL for testing.
//
// logSink is the writer to which the server logs are written. You usually want to use testhelpers.NewWriter.
// lookupEnv is a function that returns the value of an environment variable. It has same signature as [os.LookupEnv].
// run is the function that starts the server. We expect the server to log the address it's listening on to LogAddrKey.
func StartServer(
	t *testing.T,
	logSink io.Writer,
	lookupEnv func(string) (string, bool),
	run func(context.Context, *slog.Logger, func(string) (string, bool)) error,
) (*Server, error) {
	var (
		server *Server
		ctx    = t.Context()
	)
	t.Cleanup(func() {
		if server != nil {
			server.Shutdown()
		}
	})
	ctx, cancel := context.WithCancelCause(ctx)
	serverDone := make(chan struct{})

	// We need to grab the dynamically allocated port from the log output.
	addrCh := make(chan string, 1)
	// We need the sqlite DSN for the client to do database manipulation in tests.
	dsnCh := make(chan string, 1)
	logger := slog.New(logging.NewContextHandler(slog.NewTextHandler(logSink, &slog.HandlerOptions{
		AddSource: false,
		Level:     slog.LevelDebug,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == LogAddrKey {
				addrCh <- a.Value.String()
			}
			if a.Key == LogDsnKey {
				dsnCh <- a.Value.String()
			}
			return a
		},
	})))

	// Start the server and wait for it to be ready.
	go func() {
		defer close(serverDone)
		if err := run(ctx, logger, lookupEnv); err != nil {
			cancel(err)
		}
	}()
	addr := ""
	dsn := ""
	for dsn == "" || addr == "" {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled: %w", context.Cause(ctx))
		case addr = <-addrCh:
		case dsn = <-dsnCh:
		}
	}

	var (
		err    error
		client *Client
	)
	serverURL := fmt.Sprintf("http://%s", addr)
	if client, err = NewClient(serverURL, "localhost", "http://localhost:0"); err != nil {
		return nil, fmt.Errorf("new client: %w", err)
	}
	if err = client.WaitForReady(ctx, "/api/healthy"); err != nil {
		return nil, fmt.Errorf("wait for ready: %w", err)
	}
	var db *sql.DB
	db, err = sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	server = &Server{
		url:        serverURL,
		client:     client,
		db:         db,
		cancel:     cancel,
		serverDone: serverDone,
	}

	return server, nil
}

func (s *Server) Client() *Client {
	return s.client
}

func (s *Server) URL() string {
	return s.url
}

func (s *Server) DB() *sql.DB {
	return s.db
}

func (s *Server) Shutdown() {
	s.cancel(nil)
	<-s.serverDone
}
