package storage

import (
	"strings"
	"testing"
)

func TestParseS3URL(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		wantBucket string
		wantKey    string
		wantErr    bool
	}{
		{
			name:       "global endpoint",
			url:        "https://flash-card-app-images.s3.amazonaws.com/images/abc.png",
			wantBucket: "flash-card-app-images",
			wantKey:    "images/abc.png",
		},
		{
			name:       "regional endpoint",
			url:        "https://flash-card-app-images.s3.ap-southeast-1.amazonaws.com/question-images/abc.png",
			wantBucket: "flash-card-app-images",
			wantKey:    "question-images/abc.png",
		},
		{
			name:       "legacy answer bucket still parseable",
			url:        "https://flash-card-app-answer-images.s3.amazonaws.com/images/xyz.jpg",
			wantBucket: "flash-card-app-answer-images",
			wantKey:    "images/xyz.jpg",
		},
		{
			name:    "not an s3 url",
			url:     "https://example.com/images/abc.png",
			wantErr: true,
		},
		{
			name:    "missing key",
			url:     "https://bucket.s3.amazonaws.com/",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucket, key, err := parseS3URL(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseS3URL(%q) expected error, got bucket=%q key=%q", tt.url, bucket, key)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseS3URL(%q) error: %v", tt.url, err)
			}
			if bucket != tt.wantBucket || key != tt.wantKey {
				t.Errorf("parseS3URL(%q) = (%q, %q), want (%q, %q)", tt.url, bucket, key, tt.wantBucket, tt.wantKey)
			}
		})
	}
}

func TestObjectKey(t *testing.T) {
	key := objectKey(QuestionImagePrefix, "photo.png")
	if !strings.HasPrefix(key, "question-images/") {
		t.Errorf("key %q should start with question-images/", key)
	}
	if !strings.HasSuffix(key, ".png") {
		t.Errorf("key %q should keep the .png extension", key)
	}

	key = objectKey(AnswerImagePrefix, "no-extension")
	if !strings.HasPrefix(key, "answer-images/") {
		t.Errorf("key %q should start with answer-images/", key)
	}
}
