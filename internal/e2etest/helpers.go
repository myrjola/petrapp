package e2etest

import (
	"fmt"
	"github.com/PuerkitoBio/goquery"
)

// FindInputForLabel finds the input element associated with a label in the given form.
func FindInputForLabel(form *goquery.Selection, labelText string) (*goquery.Selection, error) {
	// Find the label with matching text
	label := form.Find(fmt.Sprintf("label:contains(%s)", labelText))
	if label.Length() == 0 {
		return nil, fmt.Errorf("label not found: %s", labelText)
	}

	// Get the associated input's name attribute
	var input *goquery.Selection
	if id, exists := label.Attr("for"); exists {
		// If label has 'for' attribute, find input by ID
		input = form.Find(fmt.Sprintf("#%s", id))
	} else {
		// Otherwise, find input within label
		input = label.Find("input")
	}

	if input.Length() == 0 {
		return nil, fmt.Errorf("input not found for label: %s", labelText)
	}

	return input, nil
}

// FindForm finds a form in the doc identified with action formActionUrlPath and returns the form selection.
func FindForm(doc *goquery.Document, formActionURLPath string) (*goquery.Selection, error) {
	form := doc.Find(fmt.Sprintf("form[action='%s']", formActionURLPath))
	if form.Length() == 0 {
		return nil, fmt.Errorf("form not found: %s", formActionURLPath)
	}
	return form, nil
}
