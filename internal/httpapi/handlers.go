package httpapi

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"gopkg.in/go-playground/validator.v9"

	"flashcard_lambda/internal/persistence"
)

var validate = validator.New()

// Resource provides the CRUD handlers for one entity type. ListParam is
// the query parameter naming the parent id (e.g. "categoryId"); empty
// means the resource lists without a parent (categories, tags). DeleteFn,
// when set, replaces Repo.Delete — used to hook in cascading deletes.
type Resource[T any, C any, U any] struct {
	Repo      persistence.Repository[T, C, U]
	ListParam string
	DeleteFn  func(ctx context.Context, id string) (*T, error)
}

func (res *Resource[T, C, U]) List(w http.ResponseWriter, r *http.Request) {
	parentID := ""
	if res.ListParam != "" {
		parentID = r.URL.Query().Get(res.ListParam)
		if parentID == "" {
			writeError(w, http.StatusBadRequest)
			return
		}
	}

	items, err := res.Repo.List(r.Context(), parentID)
	if err != nil {
		serverError(w, err)
		return
	}
	if items == nil {
		items = []T{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (res *Resource[T, C, U]) Get(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeError(w, http.StatusBadRequest)
		return
	}

	item, err := res.Repo.Get(r.Context(), id)
	if err != nil {
		serverError(w, err)
		return
	}
	if item == nil {
		writeError(w, http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (res *Resource[T, C, U]) Create(w http.ResponseWriter, r *http.Request) {
	var req C
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("Can't unmarshal body: %v", err)
		writeError(w, http.StatusUnprocessableEntity)
		return
	}
	if err := validate.Struct(&req); err != nil {
		log.Printf("Invalid body: %v", err)
		writeError(w, http.StatusBadRequest)
		return
	}

	item, err := res.Repo.Create(r.Context(), req)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (res *Resource[T, C, U]) Update(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeError(w, http.StatusBadRequest)
		return
	}

	var req U
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("Can't unmarshal body: %v", err)
		writeError(w, http.StatusUnprocessableEntity)
		return
	}
	if err := validate.Struct(&req); err != nil {
		log.Printf("Invalid body: %v", err)
		writeError(w, http.StatusBadRequest)
		return
	}

	item, err := res.Repo.Update(r.Context(), id, req)
	if err != nil {
		serverError(w, err)
		return
	}
	if item == nil {
		writeError(w, http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (res *Resource[T, C, U]) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeError(w, http.StatusBadRequest)
		return
	}

	deleteFn := res.DeleteFn
	if deleteFn == nil {
		deleteFn = res.Repo.Delete
	}

	item, err := deleteFn(r.Context(), id)
	if err != nil {
		serverError(w, err)
		return
	}
	if item == nil {
		writeError(w, http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, item)
}
