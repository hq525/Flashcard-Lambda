package controllers

import (
	"context"
	"encoding/json"
	"flashcard_lambda/internal/constants"
	"flashcard_lambda/internal/utils"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

type presignedURLResponse struct {
	PresignedURL string `json:"presignedUrl"`
	ImageURL     string `json:"imageUrl"`
}

func GetPresignedURL(ctx context.Context, req events.APIGatewayProxyRequest, s3Client s3.Client) (events.APIGatewayProxyResponse, error) {
	fileName, ok := req.QueryStringParameters["fileName"]
	if !ok {
		return utils.ClientError(http.StatusBadRequest)
	}

	ext := filepath.Ext(fileName)
	key := "images/" + uuid.NewString() + ext
	bucket := constants.GetBucketName()

	presignClient := s3.NewPresignClient(&s3Client)
	presignedReq, err := presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(15*time.Minute))
	if err != nil {
		return utils.ServerError(err)
	}

	imageURL := "https://" + bucket + ".s3.amazonaws.com/" + key
	log.Printf("Generated presigned URL for key = %s", key)

	body, err := json.Marshal(presignedURLResponse{
		PresignedURL: presignedReq.URL,
		ImageURL:     imageURL,
	})
	if err != nil {
		return utils.ServerError(err)
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Body:       string(body),
		Headers:    constants.CORS_HEADERS,
	}, nil
}
