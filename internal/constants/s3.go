package constants

import "os"

func GetBucketName() string {
	return os.Getenv("S3_BUCKET")
}

func GetAnswerImageBucketName() string {
	return os.Getenv("S3_ANSWER_IMAGE_BUCKET")
}
