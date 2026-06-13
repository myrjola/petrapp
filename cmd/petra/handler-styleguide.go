package main

import (
	"fmt"
	"html/template"
	"net/http"
)

// Token-scale extents from the open-props-derived design system in ui/static/main.css.
// They are inherent to the design system, not arbitrary numbers — name them so
// `mnd` (magic-number-detector) doesn't flag them as bare literals.
const (
	scaleGrayMax       = 10
	scaleStoneMax      = 10
	scaleClayMax       = 6
	scaleSizeMax       = 15
	scaleFontSizeMax   = 8
	scaleFluidFontMax  = 3
	scaleFontWeightMax = 9
	scaleRadiusMax     = 6
)

type styleguideTemplateData struct {
	BaseTemplateData

	Grays          []string
	Stones         []string
	Clays          []string
	SemanticColors []string
	// ColorTokens is the union of all color slices above. The template iterates over
	// it once to emit a `.bg-{name}` utility rule per token, avoiding inline `style=`
	// attributes (blocked by the `style-src 'nonce-...'` CSP).
	ColorTokens    []string
	Sizes          []string
	FontSizes      []string
	FluidFontSizes []string
	FontWeights    []string
	Radii          []string
	// BannerExamples drives the Banner section of the styleguide.
	BannerExamples []BannerData
	// PageHeaderExample drives the Page header section of the styleguide.
	PageHeaderExample PageHeaderData
	// FieldExamples drives the Field section of the styleguide.
	FieldExamples []FieldData
	// AdminNavExamples drives the Admin nav section of the styleguide.
	AdminNavExamples []AdminNavData
}

// styleguideGET renders the design-token reference page.
// Wired in routes.go only when app.devMode is true; returns 404 otherwise.
func (app *application) styleguideGET(w http.ResponseWriter, r *http.Request) {
	if !app.devMode {
		http.NotFound(w, r)
		return
	}

	rangeNames := func(prefix string, start, endInclusive int) []string {
		names := make([]string, 0, endInclusive-start+1)
		for i := start; i <= endInclusive; i++ {
			names = append(names, fmt.Sprintf("%s-%d", prefix, i))
		}
		return names
	}

	grays := rangeNames("gray", 0, scaleGrayMax)
	stones := rangeNames("stone", 0, scaleStoneMax)
	clays := rangeNames("clay", 0, scaleClayMax)
	semantics := []string{
		"color-surface",
		"color-surface-elevated",
		"color-surface-active",
		"color-surface-completed",
		"color-border",
		"color-border-focus",
		"color-text-primary",
		"color-text-secondary",
		"color-text-muted",
		"color-success",
		"color-success-bg",
		"color-warning",
		"color-warning-bg",
		"color-error",
		"color-error-bg",
		"color-info",
		"color-info-bg",
		"ember",
	}
	colorTokens := make([]string, 0,
		len(grays)+len(stones)+len(clays)+len(semantics))
	colorTokens = append(colorTokens, grays...)
	colorTokens = append(colorTokens, stones...)
	colorTokens = append(colorTokens, clays...)
	colorTokens = append(colorTokens, semantics...)

	base := newBaseTemplateData(r)
	data := styleguideTemplateData{
		BaseTemplateData:  base,
		Grays:             grays,
		Stones:            stones,
		Clays:             clays,
		SemanticColors:    semantics,
		ColorTokens:       colorTokens,
		Sizes:             rangeNames("size", 1, scaleSizeMax),
		FontSizes:         append([]string{"font-size-00"}, rangeNames("font-size", 0, scaleFontSizeMax)...),
		FluidFontSizes:    rangeNames("font-size-fluid", 0, scaleFluidFontMax),
		FontWeights:       rangeNames("font-weight", 1, scaleFontWeightMax),
		Radii:             append(rangeNames("radius", 1, scaleRadiusMax), "radius-round"),
		BannerExamples:    styleguideBannerExamples(base.Nonce),
		PageHeaderExample: styleguidePageHeaderExample(base.Nonce),
		FieldExamples:     styleguideFieldExamples(base.Nonce),
		AdminNavExamples:  styleguideAdminNavExamples(base.Nonce),
	}
	app.render(w, r, http.StatusOK, "styleguide", data)
}

// styleguideBannerExamples returns the three banner variants demoed on the styleguide page.
func styleguideBannerExamples(nonce template.HTMLAttr) []BannerData {
	return []BannerData{
		{
			Variant: BannerVariantError,
			Message: "Something went wrong. Please try again.",
			Nonce:   nonce,
		},
		{
			Variant: BannerVariantSuccess,
			Message: "Your changes have been saved.",
			Nonce:   nonce,
		},
		{
			Variant: BannerVariantInfo,
			Message: "Heads up — this is informational.",
			Nonce:   nonce,
		},
	}
}

// styleguidePageHeaderExample returns the page-header example demoed on the styleguide page.
func styleguidePageHeaderExample(nonce template.HTMLAttr) PageHeaderData {
	return PageHeaderData{
		Title:    "Page title",
		Subtitle: "An optional subtitle that explains the page.",
		Nonce:    nonce,
	}
}

// styleguideFieldExamples returns the field examples demoed on the styleguide page.
func styleguideFieldExamples(nonce template.HTMLAttr) []FieldData {
	return []FieldData{
		{ //nolint:exhaustruct // Styleguide example only sets the fields it demonstrates.
			Label:    "Exercise name",
			Name:     "styleguide-name",
			Type:     inputTypeText,
			Required: true,
			Hint:     "Shown to you when picking exercises.",
			Nonce:    nonce,
		},
		{ //nolint:exhaustruct // Styleguide example only sets the fields it demonstrates.
			Label: "Target reps",
			Name:  "styleguide-reps",
			Type:  inputTypeNumber,
			Value: "8",
			Min:   "1",
			Max:   "30",
			Step:  "1",
			Nonce: nonce,
		},
	}
}

// styleguideAdminNavExamples returns the admin-nav examples demoed on
// the styleguide page — one render per active section.
func styleguideAdminNavExamples(nonce template.HTMLAttr) []AdminNavData {
	return []AdminNavData{
		{Active: adminSectionExercises, Nonce: nonce},
		{Active: adminSectionFeatureFlags, Nonce: nonce},
	}
}
