package main

// BannerData is the dot for the `banner` component. Variant is one of
// "error", "success", or "info"; the component renders nothing when
// Message is empty.
type BannerData struct {
	Variant string
	Message string
}
