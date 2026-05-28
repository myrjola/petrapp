package main

import (
	"testing"

	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

func Test_application_styleguide(t *testing.T) {
	t.Parallel()

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

	// Stone and Clay ramps — the core of the Stone design direction.
	if doc.Find("h3:contains('Stone')").Length() == 0 {
		t.Error("expected a 'Stone' colour section on the styleguide")
	}
	if doc.Find(".bg-stone-5").Length() == 0 {
		t.Error("expected a --stone-5 swatch on the styleguide")
	}
	if doc.Find("h3:contains('Clay')").Length() == 0 {
		t.Error("expected a 'Clay' colour section on the styleguide")
	}
	if doc.Find(".bg-clay-4").Length() == 0 {
		t.Error("expected a --clay-4 swatch on the styleguide")
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

	// Muscle-chip — pill used by exercise-info / -add / -swap pages.
	if doc.Find("h2:contains('Muscle chip')").Length() == 0 {
		t.Error("expected a 'Muscle chip' section on the styleguide")
	}
	if doc.Find(".muscle-chip").Length() == 0 {
		t.Error("expected a .muscle-chip example on the styleguide")
	}
	if doc.Find(".muscle-chip.muscle-chip--primary").Length() == 0 {
		t.Error("expected a .muscle-chip--primary example on the styleguide")
	}

	// Rest-chip — countdown chip used by the exerciseset and workout pages.
	if doc.Find("h2:contains('Rest chip')").Length() == 0 {
		t.Error("expected a 'Rest chip' section on the styleguide")
	}
	if doc.Find(".rest-chip").Length() == 0 {
		t.Error("expected a .rest-chip example on the styleguide")
	}
	if doc.Find(".rest-chip.ready").Length() == 0 {
		t.Error("expected a .rest-chip.ready example on the styleguide")
	}

	// Sheet-dialog — slide-up modal used by exercise-add / -swap.
	if doc.Find("h2:contains('Sheet dialog')").Length() == 0 {
		t.Error("expected a 'Sheet dialog' section on the styleguide")
	}
	if doc.Find("dialog.sheet-dialog").Length() == 0 {
		t.Error("expected a dialog.sheet-dialog example on the styleguide")
	}
	if doc.Find("dialog.sheet-dialog .sheet-dialog__close").Length() == 0 {
		t.Error("expected the sheet-dialog example to render a .sheet-dialog__close row")
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

	// Page-header component.
	if doc.Find("h2:contains('Page header')").Length() == 0 {
		t.Error("expected a 'Page header' section")
	}
	if doc.Find(".page-header h1").Length() == 0 {
		t.Error("expected the page-header example to contain an h1")
	}
	if doc.Find(".page-header .page-header-subtitle").Length() == 0 {
		t.Error("expected the page-header example to contain a subtitle")
	}

	// Field component — the label/input binding is the guarantee under test.
	if doc.Find("h2:contains('Field')").Length() == 0 {
		t.Error("expected a 'Field' section")
	}
	fieldInput := doc.Find(".field input").First()
	if fieldInput.Length() == 0 {
		t.Fatal("expected the field example to contain an input")
	}
	inputID, _ := fieldInput.Attr("id")
	if inputID == "" {
		t.Error("expected the field input to have an id")
	}
	if doc.Find(".field label[for='"+inputID+"']").Length() == 0 {
		t.Errorf("expected a label bound to the input id %q", inputID)
	}
	describedBy, hasDescribedBy := fieldInput.Attr("aria-describedby")
	if !hasDescribedBy {
		t.Error("expected the field input to have aria-describedby (example has a hint)")
	}
	if describedBy != "" && doc.Find("#"+describedBy).Length() == 0 {
		t.Errorf("expected an element with id %q for aria-describedby", describedBy)
	}

	// Button variants — every variant + modifier renders on the styleguide.
	if doc.Find("h3:contains('Button variants')").Length() == 0 {
		t.Error("expected a 'Button variants' section on the styleguide")
	}
	for _, cls := range []string{
		".btn",
		".btn.btn--quiet",
		".btn.btn--ghost",
		".btn.btn--danger",
		".btn.btn--focus",
		".btn.btn--sm",
		".btn.btn--block",
	} {
		if doc.Find(cls).Length() == 0 {
			t.Errorf("expected a %s example on the styleguide", cls)
		}
	}
	// Disabled and aria-busy examples too, so the rule visibly covers them.
	if doc.Find(".btn:disabled, button.btn[disabled]").Length() == 0 {
		t.Error("expected a disabled button example on the styleguide")
	}
	if doc.Find(`.btn[aria-busy="true"]`).Length() == 0 {
		t.Error("expected an aria-busy button example on the styleguide")
	}

	// Admin nav component — both states render on the styleguide.
	if doc.Find("h2:contains('Admin nav')").Length() == 0 {
		t.Error("expected an 'Admin nav' section on the styleguide")
	}
	if doc.Find(".admin-nav").Length() < 2 {
		t.Errorf("expected at least two .admin-nav examples on the styleguide, got %d",
			doc.Find(".admin-nav").Length())
	}
	if doc.Find(`.admin-nav .admin-nav__tab[aria-current="page"][href="/admin/exercises"]`).Length() == 0 {
		t.Error("expected an Exercises-active example with aria-current=page")
	}
	if doc.Find(`.admin-nav .admin-nav__tab[aria-current="page"][href="/admin/feature-flags"]`).Length() == 0 {
		t.Error("expected a Feature-Flags-active example with aria-current=page")
	}
}
