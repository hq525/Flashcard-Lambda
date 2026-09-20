package storage

import (
	"bytes"
	"context"
	"errors"
	"path"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const readURLExpiry = 5 * time.Minute

// Allow brief browser reuse, never shared-cache storage. Signed URLs still
// expire after five minutes; API responses containing those URLs use no-store.
const imageCacheControl = "private, max-age=60, must-revalidate"

type S3ImageStore struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
}

func NewS3ImageStore(client *s3.Client, bucket string) *S3ImageStore {
	return &S3ImageStore{client: client, presign: s3.NewPresignClient(client), bucket: bucket}
}

func (s *S3ImageStore) Put(ctx context.Context, key string, data []byte, contentType string) error {
	if err := validateManagedKey(key); err != nil {
		return err
	}
	if len(data) == 0 || len(data) > MaxStoredImageBytes {
		return ErrImageTooLarge
	}
	if (path.Ext(key) == ".jpg" && contentType != "image/jpeg") || (path.Ext(key) == ".png" && contentType != "image/png") {
		return ErrInvalidImage
	}
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key), Body: bytes.NewReader(data), ContentType: aws.String(contentType), CacheControl: aws.String(imageCacheControl), ContentDisposition: aws.String("inline"), ServerSideEncryption: types.ServerSideEncryptionAes256, IfNoneMatch: aws.String("*")})
	return err
}

func (s *S3ImageStore) ReadURL(ctx context.Context, key string) (string, error) {
	if err := validateManagedKey(key); err != nil {
		return "", err
	}
	contentType := "image/png"
	if path.Ext(key) == ".jpg" {
		contentType = "image/jpeg"
	}
	result, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key), ResponseContentType: aws.String(contentType), ResponseContentDisposition: aws.String("inline"), ResponseCacheControl: aws.String(imageCacheControl)}, s3.WithPresignExpires(readURLExpiry))
	if err != nil {
		return "", err
	}
	return result.URL, nil
}

func (s *S3ImageStore) Delete(ctx context.Context, key string) error {
	if err := validateManagedKey(key); err != nil {
		return err
	}
	if s.bucket == "" {
		return errors.New("image bucket is not configured")
	}
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	return err
}
