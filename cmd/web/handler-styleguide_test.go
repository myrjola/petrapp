package main

import (
	"testing"

	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

func Test_application_styleguide(t *testing.T) {
	ctx := t.Context()

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	doc, err := client.GetDoc(ctx, "/dev/styleguide")
	if err != nil {
		t.Fatalf("Failed to get styleguide: %v", err)
	}

	// Layout primitives.
	if doc.Find("h2:contains('Layout primitives')").Length() == 0 {
		t.Error("expected a 'Layout primitives' section")
	}
	for _, cls := range []string{".stack", ".cluster", ".grid-auto", ".center"} {
		if doc.Find(cls).Length() == 0 {
			t.Errorf("expected a %s example on the styleguide", cls)
		}
	}

	// Badge and card.
	if doc.Find(".badge").Length() == 0 {
		t.Error("expected a .badge example on the styleguide")
	}
	if doc.Find(".badge.badge--success").Length() == 0 {
		t.Error("expected a .badge--success example on the styleguide")
	}
	if doc.Find(".card").Length() == 0 {
		t.Error("expected a .card example on the styleguide")
	}

	// Banner component.
	if doc.Find("h2:contains('Banner')").Length() == 0 {
		t.Error("expected a 'Banner' section")
	}
	if doc.Find(".banner.banner--error").Length() == 0 {
		t.Error("expected a .banner--error example on the styleguide")
	}
	if doc.Find(".banner.banner--error[role='alert']").Length() == 0 {
		t.Error("expected the error banner to carry role=alert")
	}
	if doc.Find(".banner.banner--success").Length() == 0 {
		t.Error("expected a .banner--success example on the styleguide")
	}
}
