package storage

import "context"

// ImageStore accepts only server-generated managed object keys. No URL supplied
// by an HTTP client can choose a bucket or deletion target.
type ImageStore interface {
	Put(ctx context.Context, key string, data []byte, contentType string) error
	ReadURL(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error
}
