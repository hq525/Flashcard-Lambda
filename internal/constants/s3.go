package constants

import "os"

func GetBucketName() string {
	return os.Getenv("S3_BUCKET")
}
