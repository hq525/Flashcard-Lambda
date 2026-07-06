package storage

import "context"

// Key prefixes inside the single image bucket.
const (
	QuestionImagePrefix = "question-images"
	AnswerImagePrefix   = "answer-images"
)

type PresignResult struct {
	// UploadURL is the presigned PUT URL the client uploads to.
	UploadURL string
	// ImageURL is the durable URL to store on the image record.
	ImageURL string
}

// ImageStore abstracts image blob storage (S3 in production).
type ImageStore interface {
	PresignUpload(ctx context.Context, prefix, fileName, contentType string) (*PresignResult, error)
	// Delete removes the object referenced by a stored image URL.
	Delete(ctx context.Context, imageURL string) error
}
