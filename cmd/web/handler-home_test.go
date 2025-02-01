package main

import (
	"context"
	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func testLookupEnv(key string) (string, bool) {
	switch key {
	case "PETRAPP_SQLITE_URL":
		return ":memory:", true
	case "PETRAPP_ADDR":
		return "localhost:0", true
	default:
		return "", false
	}
}

func Test_application_home(t *testing.T) {
	ctx := context.Background()
	server, err := e2etest.StartServer(context.Background(), os.Stdout, testLookupEnv, run)
	require.NoError(t, err)
	client := server.Client()
	doc, err := client.GetDoc(ctx, "/")
	require.NoError(t, err)
	require.Equal(t, 1, doc.Find("button:contains('Sign in')").Length())
	require.Equal(t, 1, doc.Find("button:contains('Register')").Length())
	require.Equal(t, 0, doc.Find("button:contains('Log out')").Length())

	doc, err = client.Register(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, doc.Find("button:contains('Sign in')").Length())
	require.Equal(t, 0, doc.Find("button:contains('Register')").Length())
	require.Equal(t, 1, doc.Find("button:contains('Log out')").Length())

	// Log out and log back in
	doc, err = client.Logout(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, doc.Find("button:contains('Sign in')").Length())
	require.Equal(t, 1, doc.Find("button:contains('Register')").Length())
	require.Equal(t, 0, doc.Find("button:contains('Log out')").Length())

	doc, err = client.Login(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, doc.Find("button:contains('Sign in')").Length())
	require.Equal(t, 0, doc.Find("button:contains('Register')").Length())
	require.Equal(t, 1, doc.Find("button:contains('Log out')").Length())
	// TODOs for nicer test setup:
	// - http client with
	//     - html assertion helper
	//     - form submission helper
	// - webauthn glue for setting up users for tests
}
