package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"

	"flashcard_lambda/internal/models"
	"flashcard_lambda/internal/persistence"
	"flashcard_lambda/internal/service"
)

type ReviewService interface {
	Preview(context.Context, string) (*models.ReviewOptions, error)
	Review(context.Context, string, models.ReviewCardRequest) (*models.ReviewCardResponse, error)
	History(context.Context, string) ([]models.CardReview, error)
}

func registerReviewRoutes(mux *http.ServeMux, reviews ReviewService) {
	mux.HandleFunc("GET /card-review-options", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.URL.Query().Get("cardId"))
		if id == "" {
			writeError(w, http.StatusBadRequest)
			return
		}
		options, err := reviews.Preview(r.Context(), id)
		if err != nil {
			reviewError(w, err)
			return
		}
		// Previews depend on elapsed time and must not be cached by proxies.
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, options)
	})
	mux.HandleFunc("POST /card-review", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.URL.Query().Get("cardId"))
		if id == "" {
			writeError(w, http.StatusBadRequest)
			return
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		var req models.ReviewCardRequest
		if err := decoder.Decode(&req); err != nil {
			writeError(w, http.StatusUnprocessableEntity)
			return
		}
		// Do not silently accept an extra object or trailing garbage.
		if err := decoder.Decode(new(any)); err != io.EOF {
			writeError(w, http.StatusUnprocessableEntity)
			return
		}
		validRating := req.Rating == models.RatingAgain || req.Rating == models.RatingHard || req.Rating == models.RatingGood || req.Rating == models.RatingEasy
		if strings.TrimSpace(req.ReviewId) == "" || len(req.ReviewId) > 128 || req.ExpectedRevision == nil || *req.ExpectedRevision == math.MaxUint64 || !validRating {
			writeError(w, http.StatusBadRequest)
			return
		}
		result, err := reviews.Review(r.Context(), id, req)
		if err != nil {
			reviewError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	mux.HandleFunc("GET /card-reviews", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.URL.Query().Get("cardId"))
		if id == "" {
			writeError(w, http.StatusBadRequest)
			return
		}
		items, err := reviews.History(r.Context(), id)
		if err != nil {
			reviewError(w, err)
			return
		}
		if items == nil {
			items = []models.CardReview{}
		}
		writeJSON(w, http.StatusOK, items)
	})
}

func reviewError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidReview):
		writeError(w, http.StatusBadRequest)
	case errors.Is(err, service.ErrCardNotFound):
		writeError(w, http.StatusNotFound)
	case errors.Is(err, persistence.ErrReviewConflict):
		writeJSON(w, http.StatusConflict, errorBody{Message: "This card was already reviewed or changed. Refresh its review options and try again."})
	default:
		serverError(w, err)
	}
}
