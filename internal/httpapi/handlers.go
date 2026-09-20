package httpapi

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"gopkg.in/go-playground/validator.v9"

	"flashcard_lambda/internal/persistence"
)

var validate = validator.New()

// ErrParentNotFound rejects child creation without exposing another entity's data.
var ErrParentNotFound = errors.New("referenced parent was not found")

// Resource provides the CRUD handlers for one entity type. ListParam is
// the query parameter naming the parent id (e.g. "categoryId"); empty
// means the resource lists without a parent (categories, tags). DeleteFn,
// when set, replaces Repo.Delete — used to hook in cascading deletes.
type Resource[T any, C any, U any] struct {
	Repo           persistence.Repository[T, C, U]
	ListParam      string
	DeleteFn       func(ctx context.Context, id string) (*T, error)
	ValidateCreate func(ctx context.Context, req C) error
	// Prepare may replace scalar DTO fields (such as signed image URLs). It
	// receives a shallow copy and must not mutate nested maps, slices or pointers.
	Prepare func(ctx context.Context, item *T) error
}

func (res *Resource[T, C, U]) List(w http.ResponseWriter, r *http.Request) {
	parentID := ""
	if res.ListParam != "" {
		parentID = r.URL.Query().Get(res.ListParam)
		if !validResourceID(parentID) {
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
	if res.Prepare != nil {
		items = slices.Clone(items)
		for i := range items {
			if err := res.Prepare(r.Context(), &items[i]); err != nil {
				serverError(w, err)
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, items)
}

func (res *Resource[T, C, U]) Get(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if !validResourceID(id) {
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
	res.writeItem(w, r, http.StatusOK, item)
}

func (res *Resource[T, C, U]) Create(w http.ResponseWriter, r *http.Request) {
	var req C
	if !decodeJSONRequest(w, r, &req, maxJSONRequestBytes) {
		return
	}
	if err := validate.Struct(&req); err != nil {
		writeError(w, http.StatusBadRequest)
		return
	}
	if res.ValidateCreate != nil {
		if err := res.ValidateCreate(r.Context(), req); err != nil {
			serverError(w, err)
			return
		}
	}

	item, err := res.Repo.Create(r.Context(), req)
	if err != nil {
		serverError(w, err)
		return
	}
	res.writeItem(w, r, http.StatusCreated, item)
}

func (res *Resource[T, C, U]) Update(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if !validResourceID(id) {
		writeError(w, http.StatusBadRequest)
		return
	}

	var req U
	if !decodeJSONRequest(w, r, &req, maxJSONRequestBytes) {
		return
	}
	if err := validate.Struct(&req); err != nil {
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
	res.writeItem(w, r, http.StatusOK, item)
}

func (res *Resource[T, C, U]) writeItem(w http.ResponseWriter, r *http.Request, status int, item *T) {
	if res.Prepare != nil && item != nil {
		prepared := *item
		if err := res.Prepare(r.Context(), &prepared); err != nil {
			serverError(w, err)
			return
		}
		item = &prepared
	}
	writeJSON(w, status, item)
}

func (res *Resource[T, C, U]) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if !validResourceID(id) {
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
