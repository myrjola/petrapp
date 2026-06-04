package main

import (
	"net/http"
	"strconv"
)

type listData struct {
	Todos []todoView
}

type detailData struct {
	Todo todoView
}

type todoView struct {
	ID    int
	Title string
	Notes string
	Done  bool
}

func (app *application) handleList(w http.ResponseWriter, r *http.Request) {
	items, err := app.repo.List(r.Context())
	if err != nil {
		app.serverError(w, err)
		return
	}
	views := make([]todoView, 0, len(items))
	for _, it := range items {
		views = append(views, todoView{ID: it.ID, Title: it.Title, Notes: it.Notes, Done: it.Done})
	}
	if err = app.renderer.render(w, http.StatusOK, "list", listData{Todos: views}); err != nil {
		app.serverError(w, err)
	}
}

func (app *application) handleDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	it, err := app.repo.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	data := detailData{Todo: todoView{ID: it.ID, Title: it.Title, Notes: it.Notes, Done: it.Done}}
	if err = app.renderer.render(w, http.StatusOK, "detail", data); err != nil {
		app.serverError(w, err)
	}
}

func (app *application) handleCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	title := r.PostFormValue("title")
	if title == "" {
		http.Error(w, "title required", http.StatusBadRequest)
		return
	}
	if _, err := app.repo.Create(r.Context(), title, r.PostFormValue("notes")); err != nil {
		app.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (app *application) handleToggle(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err = app.repo.Toggle(r.Context(), id); err != nil {
		app.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/todos/"+strconv.Itoa(id), http.StatusSeeOther)
}

func (app *application) handleDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err = app.repo.Delete(r.Context(), id); err != nil {
		app.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (app *application) serverError(w http.ResponseWriter, err error) {
	app.logger.Error("server error", "err", err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}
