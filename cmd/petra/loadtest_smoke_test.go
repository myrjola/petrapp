package main

import (
	"testing"

	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/loadtest"
	"github.com/myrjola/petrapp/internal/platform/testkit"
)

// Test_loadtest_scenarios_inProcess runs the stresstest scenarios against an in-process
// test server. It exists to catch UI/selector drift in `go test`, instead of waiting
// until someone runs `cmd/stresstest` against a deployed app and watches every scenario
// fail. If any of the scenarios stop matching the current templates, this test breaks.
func Test_loadtest_scenarios_inProcess(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	logger := testkit.NewLogger(testkit.NewWriter(t))

	server, err := e2etest.StartServer(t, testkit.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("start server: %v", err)
	}

	t.Run("Auth", func(t *testing.T) {
		t.Parallel()

		client, clientErr := e2etest.NewClient(server.URL(), "localhost", server.URL())
		if clientErr != nil {
			t.Fatalf("new client: %v", clientErr)
		}
		if authErr := loadtest.RunAuthFlow(ctx, client); authErr != nil {
			t.Fatalf("auth flow: %v", authErr)
		}
	})

	t.Run("WorkoutScenario", func(t *testing.T) {
		t.Parallel()

		user, regErr := loadtest.RegisterAndAuthenticateUser(ctx, server.URL(), "localhost", 1, logger)
		if regErr != nil {
			t.Fatalf("register user: %v", regErr)
		}
		if scenarioErr := loadtest.WorkoutScenario(ctx, user, logger); scenarioErr != nil {
			t.Fatalf("workout scenario: %v", scenarioErr)
		}
	})
}
