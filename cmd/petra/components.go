package main

import (
	"html/template"

	"github.com/myrjola/petrapp/internal/petra/domain"
)

// Banner variants accepted by the `banner` component. Keep aligned with
// the variant strings the template branches on.
const (
	BannerVariantError   = "error"
	BannerVariantSuccess = "success"
	BannerVariantInfo    = "info"
)

// HTML input type strings used by FieldData.Type.
const (
	inputTypeText   = "text"
	inputTypeNumber = "number"
)

// BannerData is the dot for the `banner` component. Variant is one of
// BannerVariantError, BannerVariantSuccess, or BannerVariantInfo; the
// component renders nothing when Message is empty.
//
// Live marks a banner that is the result of an action the user just took (a
// popped session flash) rather than a static reference example. Live banners
// are announced to assistive tech on load and, for the error variant, receive
// focus — a live region present in the initial HTML of a freshly-loaded
// document is otherwise NOT announced by screen readers. Leave Live false for
// styleguide / demo galleries so they don't steal focus.
type BannerData struct {
	Variant string
	Message string
	Live    bool
	Nonce   template.HTMLAttr
}

// PageHeaderData is the dot for the `page-header` component. Subtitle is
// optional and omitted from the output when empty.
type PageHeaderData struct {
	Title    string
	Subtitle string
	Nonce    template.HTMLAttr
}

// FieldData is the dot for the `field` component — a labelled single text
// input. Name is used as both the input's id and its name attribute. Type
// is an HTML input type ("text", "number", "email", ...). Min, Max, Step
// and Pattern are native-validation attributes, passed through verbatim and
// omitted from the output when empty (they are strings so "0" can be set
// explicitly). Hint, when set, is rendered and wired via aria-describedby.
//
// Error, when non-empty, marks the input invalid: aria-invalid="true", an
// error message wired into aria-describedby, and a non-colour-only treatment
// (error border + a leading marker glyph) per the design-system rule "state is
// never colour-only". The `select` and `textarea` components share this error
// contract so a form mixes control types without each re-inventing it.
type FieldData struct {
	Label    string
	Name     string
	Type     string
	Value    string
	Required bool
	Hint     string
	Error    string
	Min      string
	Max      string
	Step     string
	Pattern  string
	Nonce    template.HTMLAttr
}

// selectOption is one <option> of a select, with its selected state resolved
// by the handler. Shared by the `select` component and its callers.
type selectOption struct {
	Value    string
	Label    string
	Selected bool
}

// SelectData is the dot for the `select` component — a labelled <select>,
// single or (with Multiple) multi-value. Options carry their own Selected
// state. Hint and Error share the field component's contract (label↔id,
// aria-describedby, aria-invalid + error span). Name is both id and name.
type SelectData struct {
	Label    string
	Name     string
	Options  []selectOption
	Multiple bool
	Required bool
	Hint     string
	Error    string
	Nonce    template.HTMLAttr
}

// TextareaData is the dot for the `textarea` component — a labelled multi-line
// input. Value is the body; Rows sizes it (a string like FieldData.Min, so it
// can be omitted with ""). Hint and Error share the field component's contract.
// Name is both id and name.
type TextareaData struct {
	Label string
	Name  string
	Value string
	Rows  string
	Hint  string
	Error string
	Nonce template.HTMLAttr
}

// ErrorSummaryData is the dot for the `error-summary` component — the GOV.UK
// error summary: a role="alert", focus-on-load panel listing every field error
// as a link to #<fieldname> (so it jumps to the offending input), plus any
// form-level messages. Renders nothing when Items and Form are both empty.
// Live mirrors BannerData.Live (focus on load); leave it false on styleguide
// demos so they don't steal focus.
type ErrorSummaryData struct {
	Title string
	Items []ErrorSummaryItem
	Form  []string
	Live  bool
	Nonce template.HTMLAttr
}

// ErrorSummaryItem is one row of the error summary: a message and the field
// name it anchors to (Anchor == the input id == its form name attribute).
type ErrorSummaryItem struct {
	Anchor  string
	Message string
}

// ExerciseResultCardData drives the components/exercise-result-card partial,
// shared by the Add and Swap exercise pages. The template renders the
// exercise's structured content (Instructions, CommonMistakes, Resources)
// directly off Exercise.
type ExerciseResultCardData struct {
	Exercise    domain.Exercise
	FormAction  string // POST target for the add/swap form
	FieldName   string // hidden input name ("exercise_id" or "new_exercise_id")
	ButtonLabel string // submit button text
	Nonce       template.HTMLAttr
}

// BackLinkData is the dot for the `back-link` component.
type BackLinkData struct {
	Href  string
	Nonce template.HTMLAttr
}

// ExerciseSearchData is the dot for the `exercise-search` component.
type ExerciseSearchData struct {
	Query string
	Nonce template.HTMLAttr
}

// AdminNavData is the dot for the `admin-nav` component. Active is the
// section-name string ("exercises" or "feature-flags") used to mark the
// matching tab with aria-current="page".
type AdminNavData struct {
	Active string
	Nonce  template.HTMLAttr
}

// Admin section names accepted by AdminNavData.Active. Keep aligned with
// the values the admin-nav template branches on.
const (
	adminSectionExercises    = "exercises"
	adminSectionFeatureFlags = "feature-flags"
)
