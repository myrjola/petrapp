package main

import (
	"html/template"

	"github.com/myrjola/petrapp/internal/domain"
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
type BannerData struct {
	Variant string
	Message string
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
type FieldData struct {
	Label    string
	Name     string
	Type     string
	Value    string
	Required bool
	Hint     string
	Min      string
	Max      string
	Step     string
	Pattern  string
	Nonce    template.HTMLAttr
}

// ExerciseResultCardData drives the components/exercise-result-card partial,
// shared by the Add and Swap exercise pages. DescriptionHTML carries the
// pre-rendered markdown description so templates don't need to invoke the
// markdown pipeline at render time.
type ExerciseResultCardData struct {
	Exercise        domain.Exercise
	FormAction      string        // POST target for the add/swap form
	FieldName       string        // hidden input name ("exercise_id" or "new_exercise_id")
	ButtonLabel     string        // submit button text
	DescriptionHTML template.HTML // pre-rendered exercise description markdown
	Nonce           template.HTMLAttr
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
