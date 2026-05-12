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

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

func GetTags(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	log.Printf("Received GET tags request")
	tagDAO := persistence.NewTagDataAccessObject(&db)
	tags, err := tagDAO.GetTags(ctx, req.RequestContext.Stage)
	if err != nil {
		return utils.ServerError(err)
	}
	log.Printf("Retrieved %d tags", len(tags))

	body, err := json.Marshal(tags)
	if err != nil {
		return utils.ServerError(err)
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Body:       string(body),
		Headers:    constants.CORS_HEADERS,
	}, nil
}

func GetTag(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	id, ok := req.QueryStringParameters["id"]
	if !ok {
		return utils.ClientError(http.StatusBadRequest)
	}
	log.Printf("Received GET tag request with id = %s", id)

	tagDAO := persistence.NewTagDataAccessObject(&db)
	tag, err := tagDAO.GetTag(ctx, id, req.RequestContext.Stage)
	if err != nil {
		return utils.ServerError(err)
	}

	body, err := json.Marshal(tag)
	if err != nil {
		return utils.ServerError(err)
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Body:       string(body),
		Headers:    constants.CORS_HEADERS,
	}, nil
}

func CreateNewTag(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	var createTagRequest models.CreateTagRequest
	err := json.Unmarshal([]byte(req.Body), &createTagRequest)
	if err != nil {
		log.Printf("Can't unmarshal body: %v", err)
		return utils.ClientError(http.StatusUnprocessableEntity)
	}

	err = validate.Struct(&createTagRequest)
	if err != nil {
		log.Printf("Invalid body: %v", err)
		return utils.ClientError(http.StatusBadRequest)
	}
	log.Printf("Received POST request with new tag: %+v", createTagRequest)

	tagDAO := persistence.NewTagDataAccessObject(&db)
	res, err := tagDAO.InsertTag(ctx, createTagRequest, req.RequestContext.Stage)
	if err != nil {
		return utils.ServerError(err)
	}
	log.Printf("Inserted new tag: %+v", res)

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

func UpdateTag(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	id, ok := req.QueryStringParameters["id"]
	if !ok {
		return utils.ClientError(http.StatusBadRequest)
	}

	var updateTagRequest models.UpdateTagRequest
	err := json.Unmarshal([]byte(req.Body), &updateTagRequest)
	if err != nil {
		log.Printf("Can't unmarshal body: %v", err)
		return utils.ClientError(http.StatusUnprocessableEntity)
	}
	log.Printf("Received PUT request with tag: %+v", updateTagRequest)

	tagDAO := persistence.NewTagDataAccessObject(&db)
	res, err := tagDAO.UpdateTag(ctx, id, updateTagRequest, req.RequestContext.Stage)
	if err != nil {
		return utils.ServerError(err)
	}

	if res == nil {
		return utils.ClientError(http.StatusNotFound)
	}

	log.Printf("Updated tag: %+v", res)

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

func DeleteTag(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	id, ok := req.QueryStringParameters["id"]
	if !ok {
		return utils.ClientError(http.StatusBadRequest)
	}
	log.Printf("Received DELETE request with id = %s", id)

	tagDAO := persistence.NewTagDataAccessObject(&db)
	tag, err := tagDAO.DeleteTag(ctx, id, req.RequestContext.Stage)
	if err != nil {
		return utils.ServerError(err)
	}

	if tag == nil {
		return utils.ClientError(http.StatusNotFound)
	}

	body, err := json.Marshal(tag)
	if err != nil {
		return utils.ServerError(err)
	}
	log.Printf("Successfully deleted tag %+v", tag)

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Body:       string(body),
		Headers:    constants.CORS_HEADERS,
	}, nil
}
