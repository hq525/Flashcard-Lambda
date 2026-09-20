package storage

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type securityHTTPClient func(*http.Request) (*http.Response, error)

func (f securityHTTPClient) Do(r *http.Request) (*http.Response, error) { return f(r) }

func TestDeleteRejectsUntrustedTargetsBeforeAWS(t *testing.T) {
	for _, target := range []string{
		"https://legacy.s3.amazonaws.com/private.png",
		"images/../private.png", "images/not-a-uuid.png", "answer-images/x.png", "",
	} {
		t.Run(target, func(t *testing.T) {
			calls := 0
			client := s3.New(s3.Options{Region: "us-east-1", Credentials: aws.AnonymousCredentials{}, HTTPClient: securityHTTPClient(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: 204, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
			})})
			store := NewS3ImageStore(client, "configured-bucket")
			if err := store.Delete(context.Background(), target); err == nil || calls != 0 {
				t.Fatalf("untrusted target accepted: error=%v AWS calls=%d", err, calls)
			}
		})
	}
}
