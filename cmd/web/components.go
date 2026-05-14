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
