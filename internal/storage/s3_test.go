package storage

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestManagedDeleteUsesOnlyConfiguredBucket(t *testing.T) {
	var request *http.Request
	client := s3.New(s3.Options{Region: "ap-southeast-1", Credentials: aws.AnonymousCredentials{}, HTTPClient: securityHTTPClient(func(r *http.Request) (*http.Response, error) {
		request = r
		return &http.Response{StatusCode: 204, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})})
	store := NewS3ImageStore(client, "managed-bucket")
	key := "images/e3d4a94b-f0e9-46af-a2c0-2c02850b539a.png"
	if err := store.Delete(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	if request.Method != "DELETE" || request.URL.Host != "managed-bucket.s3.ap-southeast-1.amazonaws.com" || request.URL.Path != "/"+key {
		t.Fatalf("wrong delete target %s", request.URL)
	}
}

func TestPrivateReadURLExpiresAndCannotChangeResponseType(t *testing.T) {
	client := s3.New(s3.Options{Region: "ap-southeast-1", Credentials: credentials.NewStaticCredentialsProvider("test-id", "test-secret", ""), HTTPClient: securityHTTPClient(func(*http.Request) (*http.Response, error) { t.Fatal("unexpected network I/O"); return nil, nil })})
	store := NewS3ImageStore(client, "managed-bucket")
	result, err := store.ReadURL(context.Background(), "images/e3d4a94b-f0e9-46af-a2c0-2c02850b539a.jpg")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(result)
	if u.Query().Get("X-Amz-Expires") != "300" || u.Query().Get("response-content-type") != "image/jpeg" || u.Query().Get("response-cache-control") != "private, max-age=60, must-revalidate" {
		t.Fatalf("unsafe signed read parameters: %v", u.Query())
	}
	if _, err := store.ReadURL(context.Background(), "https://elsewhere.s3.amazonaws.com/a.png"); err == nil {
		t.Fatal("arbitrary URL accepted")
	}
}

func TestManagedUploadStoresShortPrivateCachingWithoutAllowingOverwrite(t *testing.T) {
	var request *http.Request
	client := s3.New(s3.Options{Region: "ap-southeast-1", Credentials: aws.AnonymousCredentials{}, HTTPClient: securityHTTPClient(func(r *http.Request) (*http.Response, error) {
		request = r
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})})
	store := NewS3ImageStore(client, "managed-bucket")
	if err := store.Put(context.Background(), "images/e3d4a94b-f0e9-46af-a2c0-2c02850b539a.png", []byte("normalized-image"), "image/png"); err != nil {
		t.Fatal(err)
	}
	if got := request.Header.Get("Cache-Control"); got != "private, max-age=60, must-revalidate" {
		t.Fatalf("object cache policy = %q; want short browser-only caching", got)
	}
	if request.Header.Get("If-None-Match") != "*" {
		t.Fatal("cached image key could be overwritten")
	}
	if request.Header.Get("X-Amz-Acl") != "" {
		t.Fatal("private upload must not set a public ACL")
	}
}
