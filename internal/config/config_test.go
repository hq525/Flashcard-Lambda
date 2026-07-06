package config

import "testing"

func TestLoad(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "flash-card-app-dev")
	t.Setenv("S3_BUCKET", "flash-card-app-images-dev")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.TableName != "flash-card-app-dev" {
		t.Errorf("TableName = %q, want %q", cfg.TableName, "flash-card-app-dev")
	}
	if cfg.Bucket != "flash-card-app-images-dev" {
		t.Errorf("Bucket = %q, want %q", cfg.Bucket, "flash-card-app-images-dev")
	}
}

func TestLoadMissingTable(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "")
	t.Setenv("S3_BUCKET", "bucket")

	if _, err := Load(); err == nil {
		t.Fatal("Load() with missing DYNAMODB_TABLE should return an error")
	}
}

func TestLoadMissingBucket(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "table")
	t.Setenv("S3_BUCKET", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() with missing S3_BUCKET should return an error")
	}
}
