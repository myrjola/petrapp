package main

import (
	"context"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/myrjola/petrapp/internal/testhelpers"
)

// BenchmarkRenderHome measures the cost of rendering the cached home
// template into an io.Discard writer with a representative
// homeTemplateData payload. Used as a regression guard for the
// template clone-removal work.
func BenchmarkRenderHome(b *testing.B) {
	templatePath, err := filepath.Abs(filepath.Join("..", "..", "ui", "templates"))
	if err != nil {
		b.Fatalf("resolve template path: %v", err)
	}
	app := &application{ //nolint:exhaustruct // only the fields touched by pageTemplate matter here.
		templateFS:      os.DirFS(templatePath),
		parsedTemplates: newTemplateCache(),
		logger:          testhelpers.NewLogger(io.Discard),
		devMode:         false,
	}
	// Prime the cache.
	if _, err = app.pageTemplate("home"); err != nil {
		b.Fatalf("prime pageTemplate: %v", err)
	}
	data := homeTemplateData{ //nolint:exhaustruct // benchmark uses the zero-valued unauthenticated path.
		BaseTemplateData: BaseTemplateData{ //nolint:exhaustruct // zero-valued unauth payload.
			Nonce: template.HTMLAttr(`nonce="benchmark"`),
		},
	}
	ctx := context.Background()
	b.ResetTimer()
	for range b.N {
		buf, rerr := app.renderToBuf(ctx, "home", data)
		if rerr != nil {
			b.Fatalf("render: %v", rerr)
		}
		putRenderBuf(buf)
	}
}
