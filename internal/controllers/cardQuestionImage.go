package controllers

import (
	"context"
	"encoding/json"
	"flashcard_lambda/internal/constants"
	"flashcard_lambda/internal/models"
	"flashcard_lambda/internal/persistence"
	"flashcard_lambda/internal/utils"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func GetCardQuestionImages(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	cardId, ok := req.QueryStringParameters["cardId"]
	if !ok {
		return utils.ClientError(http.StatusBadRequest)
	}
	log.Printf("Received GET card question images request for cardId = %s", cardId)

	dao := persistence.NewCardQuestionImageDataAccessObject(&db)
	images, err := dao.GetCardQuestionImages(ctx, cardId)
	if err != nil {
		return utils.ServerError(err)
	}
	log.Printf("Retrieved %d card question images", len(images))

	body, err := json.Marshal(images)
	if err != nil {
		return utils.ServerError(err)
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Body:       string(body),
		Headers:    constants.CORS_HEADERS,
	}, nil
}

func GetCardQuestionImage(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	id, ok := req.QueryStringParameters["id"]
	if !ok {
		return utils.ClientError(http.StatusBadRequest)
	}
	log.Printf("Received GET card question image request with id = %s", id)

	dao := persistence.NewCardQuestionImageDataAccessObject(&db)
	image, err := dao.GetCardQuestionImage(ctx, id)
	if err != nil {
		return utils.ServerError(err)
	}

	body, err := json.Marshal(image)
	if err != nil {
		return utils.ServerError(err)
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Body:       string(body),
		Headers:    constants.CORS_HEADERS,
	}, nil
}

func CreateNewCardQuestionImage(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	var createReq models.CreateCardQuestionImageRequest
	err := json.Unmarshal([]byte(req.Body), &createReq)
	if err != nil {
		log.Printf("Can't unmarshal body: %v", err)
		return utils.ClientError(http.StatusUnprocessableEntity)
	}

	err = validate.Struct(&createReq)
	if err != nil {
		log.Printf("Invalid body: %v", err)
		return utils.ClientError(http.StatusBadRequest)
	}
	log.Printf("Received POST request with new card question image: %+v", createReq)

	dao := persistence.NewCardQuestionImageDataAccessObject(&db)
	res, err := dao.InsertCardQuestionImage(ctx, createReq)
	if err != nil {
		return utils.ServerError(err)
	}
	log.Printf("Inserted new card question image: %+v", res)

	body, err := json.Marshal(res)
	if err != nil {
		return utils.ServerError(err)
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusCreated,
		Body:       string(body),
		Headers:    constants.CORS_HEADERS,
	}, nil
}

func UpdateCardQuestionImage(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	id, ok := req.QueryStringParameters["id"]
	if !ok {
		return utils.ClientError(http.StatusBadRequest)
	}

	var updateReq models.UpdateCardQuestionImageRequest
	err := json.Unmarshal([]byte(req.Body), &updateReq)
	if err != nil {
		log.Printf("Can't unmarshal body: %v", err)
		return utils.ClientError(http.StatusUnprocessableEntity)
	}
	log.Printf("Received PUT request with card question image: %+v", updateReq)

	dao := persistence.NewCardQuestionImageDataAccessObject(&db)
	res, err := dao.UpdateCardQuestionImage(ctx, id, updateReq)
	if err != nil {
		return utils.ServerError(err)
	}

	if res == nil {
		return utils.ClientError(http.StatusNotFound)
	}

	log.Printf("Updated card question image: %+v", res)

	body, err := json.Marshal(res)
	if err != nil {
		return utils.ServerError(err)
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Body:       string(body),
		Headers:    constants.CORS_HEADERS,
	}, nil
}

func DeleteCardQuestionImage(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client, s3Client s3.Client) (events.APIGatewayProxyResponse, error) {
	id, ok := req.QueryStringParameters["id"]
	if !ok {
		return utils.ClientError(http.StatusBadRequest)
	}
	log.Printf("Received DELETE request with id = %s", id)

	dao := persistence.NewCardQuestionImageDataAccessObject(&db)
	image, err := dao.DeleteCardQuestionImage(ctx, id)
	if err != nil {
		return utils.ServerError(err)
	}

	if image == nil {
		return utils.ClientError(http.StatusNotFound)
	}

	parsed, err := url.Parse(image.ImageURL)
	if err != nil {
		return utils.ServerError(err)
	}
	key := strings.TrimPrefix(parsed.Path, "/")
	_, err = s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(constants.GetBucketName()),
		Key:    aws.String(key),
	})
	if err != nil {
		return utils.ServerError(err)
	}
	log.Printf("Deleted S3 object %s", key)

	body, err := json.Marshal(image)
	if err != nil {
		return utils.ServerError(err)
	}
	log.Printf("Successfully deleted card question image %+v", image)

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Body:       string(body),
		Headers:    constants.CORS_HEADERS,
	}, nil
}
