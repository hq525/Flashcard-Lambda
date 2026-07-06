package storage

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

const presignExpiry = 15 * time.Minute

type S3ImageStore struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
}

func NewS3ImageStore(client *s3.Client, bucket string) *S3ImageStore {
	return &S3ImageStore{
		client:  client,
		presign: s3.NewPresignClient(client),
		bucket:  bucket,
	}
}

func objectKey(prefix, fileName string) string {
	return prefix + "/" + uuid.NewString() + filepath.Ext(fileName)
}

func (s *S3ImageStore) PresignUpload(ctx context.Context, prefix, fileName, contentType string) (*PresignResult, error) {
	key := objectKey(prefix, fileName)

	presignedReq, err := s.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}, s3.WithPresignExpires(presignExpiry))
	if err != nil {
		return nil, err
	}

	return &PresignResult{
		UploadURL: presignedReq.URL,
		ImageURL:  "https://" + s.bucket + ".s3.amazonaws.com/" + key,
	}, nil
}

func (s *S3ImageStore) Delete(ctx context.Context, imageURL string) error {
	bucket, key, err := parseS3URL(imageURL)
	if err != nil {
		return err
	}

	_, err = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	return err
}

// parseS3URL extracts bucket and key from a virtual-hosted-style S3 URL
// (https://<bucket>.s3.amazonaws.com/<key> or
// https://<bucket>.s3.<region>.amazonaws.com/<key>). The bucket comes from
// the URL rather than config so records created under the old two-bucket
// layout can still be deleted.
func parseS3URL(imageURL string) (bucket, key string, err error) {
	parsed, err := url.Parse(imageURL)
	if err != nil {
		return "", "", err
	}

	host := parsed.Hostname()
	idx := strings.Index(host, ".s3")
	if idx < 1 || !strings.HasSuffix(host, ".amazonaws.com") {
		return "", "", fmt.Errorf("not a virtual-hosted S3 URL: %s", imageURL)
	}
	bucket = host[:idx]

	key = strings.TrimPrefix(parsed.Path, "/")
	if key == "" {
		return "", "", fmt.Errorf("S3 URL has no object key: %s", imageURL)
	}

	return bucket, key, nil
}
