package main

import (
	"net/http"
)

type preferencesTemplateData struct {
	BaseTemplateData
}

func (app *application) preferences(w http.ResponseWriter, r *http.Request) {
	data := preferencesTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
	}

	app.render(w, r, http.StatusOK, "preferences", data)
}
