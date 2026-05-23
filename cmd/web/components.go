package main

import "github.com/myrjola/petrapp/internal/domain"

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
}

// PageHeaderData is the dot for the `page-header` component. Subtitle is
// optional and omitted from the output when empty.
type PageHeaderData struct {
	Title    string
	Subtitle string
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
}

// ExerciseResultCardData drives the components/exercise-result-card partial,
// shared by the Add and Swap exercise pages.
type ExerciseResultCardData struct {
	Exercise    domain.Exercise
	FormAction  string // POST target for the add/swap form
	FieldName   string // hidden input name ("exercise_id" or "new_exercise_id")
	ButtonLabel string // submit button text
}
