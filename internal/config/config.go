package config

import (
	"fmt"
	"os"
)

type Config struct {
	TableName string
	Bucket    string
}

func Load() (Config, error) {
	cfg := Config{
		TableName: os.Getenv("DYNAMODB_TABLE"),
		Bucket:    os.Getenv("S3_BUCKET"),
	}
	if cfg.TableName == "" {
		return Config{}, fmt.Errorf("environment variable DYNAMODB_TABLE is not set")
	}
	if cfg.Bucket == "" {
		return Config{}, fmt.Errorf("environment variable S3_BUCKET is not set")
	}
	return cfg, nil
}
