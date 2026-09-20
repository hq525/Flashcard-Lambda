package httpapi

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"flashcard_lambda/internal/models"
	"flashcard_lambda/internal/persistence"
	"flashcard_lambda/internal/storage"
	"github.com/google/uuid"
)

type UploadBudget interface {
	ConsumeUpload(context.Context, int64) error
}

func prepareQuestionImage(images storage.ImageStore) func(context.Context, *models.CardQuestionImage) error {
	return func(ctx context.Context, image *models.CardQuestionImage) error {
		image.ImageURL = "" // Never return a legacy URL, even during migration.
		if image.StorageKey == "" {
			return nil
		}
		if err := storage.ValidateImageKey(image.Id, image.StorageKey); err != nil {
			return err
		}
		var err error
		image.ImageURL, err = images.ReadURL(ctx, image.StorageKey)
		return err
	}
}
func prepareSectionImage(images storage.ImageStore) func(context.Context, *models.CardAnswerSectionImage) error {
	return func(ctx context.Context, image *models.CardAnswerSectionImage) error {
		image.ImageURL = ""
		if image.StorageKey == "" {
			return nil
		}
		if err := storage.ValidateImageKey(image.Id, image.StorageKey); err != nil {
			return err
		}
		var err error
		image.ImageURL, err = images.ReadURL(ctx, image.StorageKey)
		return err
	}
}

func registerImageUploads(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("POST /card-question-image", uploadImage(d, "cardId", func(ctx context.Context, id string) (bool, error) {
		item, err := d.Cards.Get(ctx, id)
		return item != nil, err
	}, func(ctx context.Context, id, parent, key string, seq uint16) (any, error) {
		item, err := d.QuestionImages.Create(ctx, models.CreateCardQuestionImageRequest{Id: id, CardId: parent, SequenceNumber: seq, StorageKey: key})
		if err != nil {
			return nil, err
		}
		if item == nil {
			return nil, errors.New("image create returned no record")
		}
		if err := prepareQuestionImage(d.Images)(ctx, item); err != nil {
			return nil, err
		}
		return item, nil
	}))
	mux.HandleFunc("POST /card-answer-section-image", uploadImage(d, "cardAnswerSectionId", func(ctx context.Context, id string) (bool, error) {
		item, err := d.Sections.Get(ctx, id)
		return item != nil, err
	}, func(ctx context.Context, id, parent, key string, seq uint16) (any, error) {
		item, err := d.SectionImages.Create(ctx, models.CreateCardAnswerSectionImageRequest{Id: id, CardAnswerSectionId: parent, SequenceNumber: seq, StorageKey: key})
		if err != nil {
			return nil, err
		}
		if item == nil {
			return nil, errors.New("image create returned no record")
		}
		if err := prepareSectionImage(d.Images)(ctx, item); err != nil {
			return nil, err
		}
		return item, nil
	}))
	mux.HandleFunc("GET /presigned-url", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusGone, errorBody{Message: "Direct uploads have been retired. Reload the updated application."})
	})
}

func uploadImage(d Deps, parentParam string, exists func(context.Context, string) (bool, error), create func(context.Context, string, string, string, uint16) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || !storage.SupportedImageType(contentType) {
			writeError(w, http.StatusUnsupportedMediaType)
			return
		}
		parent := strings.TrimSpace(r.URL.Query().Get(parentParam))
		sequence, err := strconv.ParseUint(r.URL.Query().Get("sequenceNumber"), 10, 16)
		if parent == "" || len(parent) > 128 || err != nil || sequence == 0 {
			writeError(w, http.StatusBadRequest)
			return
		}
		if r.ContentLength > storage.MaxUploadBytes {
			writeError(w, http.StatusRequestEntityTooLarge)
			return
		}
		found, err := exists(r.Context(), parent)
		if err != nil {
			serverError(w, err)
			return
		}
		if !found {
			writeError(w, http.StatusNotFound)
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, storage.MaxUploadBytes))
		if err != nil {
			var large *http.MaxBytesError
			if errors.As(err, &large) {
				writeError(w, http.StatusRequestEntityTooLarge)
			} else {
				writeError(w, http.StatusBadRequest)
			}
			return
		}
		normalized, err := storage.NormalizeImage(raw, contentType)
		if err != nil {
			if errors.Is(err, storage.ErrImageTooLarge) {
				writeError(w, http.StatusRequestEntityTooLarge)
			} else {
				writeJSON(w, http.StatusUnprocessableEntity, errorBody{Message: "Upload a valid PNG, JPEG, GIF or WebP image."})
			}
			return
		}
		if d.UploadBudget == nil {
			serverError(w, errors.New("upload budget is not configured"))
			return
		}
		if err := d.UploadBudget.ConsumeUpload(r.Context(), int64(len(normalized.Data))); err != nil {
			if errors.Is(err, persistence.ErrUploadQuota) {
				writeJSON(w, http.StatusTooManyRequests, errorBody{Message: "The daily image upload limit has been reached. Try again tomorrow."})
			} else {
				serverError(w, err)
			}
			return
		}
		id := uuid.NewString()
		key, err := storage.ImageKey(id, normalized.ContentType)
		if err != nil {
			serverError(w, err)
			return
		}
		if err := d.Images.Put(r.Context(), key, normalized.Data, normalized.ContentType); err != nil {
			serverError(w, err)
			return
		}
		item, err := create(r.Context(), id, parent, key, uint16(sequence))
		// A DynamoDB timeout can hide a successful write. Retain the private object
		// on uncertainty rather than deleting the content of a committed record.
		if err != nil {
			serverError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, item)
	}
}
