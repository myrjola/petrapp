package main

// BannerData is the dot for the `banner` component. Variant is one of
// "error", "success", or "info"; the component renders nothing when
// Message is empty.
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
