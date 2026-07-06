package httpapi

import (
	"net/http"
	"strings"

	"flashcard_lambda/internal/storage"
)

type presignedURLResponse struct {
	PresignedURL string `json:"presignedUrl"`
	ImageURL     string `json:"imageUrl"`
}

// getPresignedURL hands the client a presigned PUT URL. contentType is
// required and must be an image type so the signed upload can't be used
// for arbitrary files.
func getPresignedURL(images storage.ImageStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		fileName := query.Get("fileName")
		contentType := query.Get("contentType")
		if fileName == "" || !strings.HasPrefix(contentType, "image/") {
			writeError(w, http.StatusBadRequest)
			return
		}

		prefix := storage.QuestionImagePrefix
		if query.Get("imageType") == "answer" {
			prefix = storage.AnswerImagePrefix
		}

		result, err := images.PresignUpload(r.Context(), prefix, fileName, contentType)
		if err != nil {
			serverError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, presignedURLResponse{
			PresignedURL: result.UploadURL,
			ImageURL:     result.ImageURL,
		})
	}
}
