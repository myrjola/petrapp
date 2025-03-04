package e2etest

import (
	"context"
	"fmt"
	"github.com/myrjola/petrapp/internal/logging"
	"io"
	"log/slog"
)

type Server struct {
	url    string
	client *Client
}

// LogAddrKey is the key used to log the address the server is listening on.
const LogAddrKey = "addr"

// StartServer starts the test server, waits for it to be ready, and return the server URL for testing.
//
// logSink is the writer to which the server logs are written. You usually want to use [io.Discard].
// lookupEnv is a function that returns the value of an environment variable. It has same signature as [os.LookupEnv].
// run is the function that starts the server. We expect the server to log the address it's listening on to a.
func StartServer(
	ctx context.Context,
	logSink io.Writer,
	lookupEnv func(string) (string, bool),
	run func(context.Context, *slog.Logger, func(string) (string, bool)) error,
) (*Server, error) {
	ctx, cancel := context.WithCancelCause(ctx)

	// We need to grab the dynamically allocated port from the log output.
	addrCh := make(chan string, 1)
	logger := slog.New(logging.NewContextHandler(slog.NewTextHandler(logSink, &slog.HandlerOptions{
		AddSource: false,
		Level:     slog.LevelDebug,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == LogAddrKey {
				addrCh <- a.Value.String()
			}
			return a
		},
	})))

	// Start the server and wait for it to be ready.
	go func() {
		if err := run(ctx, logger, lookupEnv); err != nil {
			cancel(err)
		}
	}()
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("context cancelled: %w", context.Cause(ctx))
	case addr := <-addrCh:
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
		return &Server{
			url:    serverURL,
			client: client,
		}, nil
	}
}

func (s *Server) Client() *Client {
	return s.client
}

func (s *Server) URL() string {
	return s.url
}
