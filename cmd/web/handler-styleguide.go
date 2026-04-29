package main

import (
	"fmt"
	"net/http"
)

// Token-scale extents from the open-props-derived design system in ui/static/main.css.
// They are inherent to the design system, not arbitrary numbers — name them so
// `mnd` (magic-number-detector) doesn't flag them as bare literals.
const (
	scaleGrayMax       = 10
	scaleSkyMax        = 10
	scaleLimeMax       = 10
	scaleRedMax        = 12
	scaleYellowMax     = 12
	scaleSizeMax       = 15
	scaleFontSizeMax   = 8
	scaleFluidFontMax  = 3
	scaleFontWeightMax = 9
	scaleRadiusMax     = 6
)

type styleguideTemplateData struct {
	BaseTemplateData
	Grays          []string
	Skies          []string
	Limes          []string
	Reds           []string
	Yellows        []string
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
	skies := rangeNames("sky", 0, scaleSkyMax)
	limes := rangeNames("lime", 0, scaleLimeMax)
	reds := rangeNames("red", 0, scaleRedMax)
	yellows := rangeNames("yellow", 0, scaleYellowMax)
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
		"color-info",
		"color-info-bg",
	}
	colorTokens := make([]string, 0, len(grays)+len(skies)+len(limes)+len(reds)+len(yellows)+len(semantics))
	colorTokens = append(colorTokens, grays...)
	colorTokens = append(colorTokens, skies...)
	colorTokens = append(colorTokens, limes...)
	colorTokens = append(colorTokens, reds...)
	colorTokens = append(colorTokens, yellows...)
	colorTokens = append(colorTokens, semantics...)

	data := styleguideTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		Grays:            grays,
		Skies:            skies,
		Limes:            limes,
		Reds:             reds,
		Yellows:          yellows,
		SemanticColors:   semantics,
		ColorTokens:      colorTokens,
		Sizes:            rangeNames("size", 1, scaleSizeMax),
		FontSizes:        append([]string{"font-size-00"}, rangeNames("font-size", 0, scaleFontSizeMax)...),
		FluidFontSizes:   rangeNames("font-size-fluid", 0, scaleFluidFontMax),
		FontWeights:      rangeNames("font-weight", 1, scaleFontWeightMax),
		Radii:            append(rangeNames("radius", 1, scaleRadiusMax), "radius-round"),
	}
	app.render(w, r, http.StatusOK, "styleguide", data)
}
