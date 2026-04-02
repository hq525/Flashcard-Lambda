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
	"gopkg.in/go-playground/validator.v9"
)

func GetCategories(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	categoryDAO := persistence.NewCategoryDataAccessObject(&db)
	categories, err := categoryDAO.GetCategories(ctx, req.RequestContext.Stage)
	if err != nil {
		return utils.ServerError(err)
	}

	body, err := json.Marshal(categories)
	if err != nil {
		return utils.ServerError(err)
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Body:       string(body),
		Headers:    constants.CORS_HEADERS,
	}, nil
}

func GetCategory(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	id, ok := req.QueryStringParameters["id"]
	if !ok {
		return utils.ClientError(http.StatusBadRequest)
	}
	log.Printf("Received GET profile request with id = %s", id)

	categoryDAO := persistence.NewCategoryDataAccessObject(&db)
	category, err := categoryDAO.GetCategory(ctx, id, req.RequestContext.Stage)
	if err != nil {
		return utils.ServerError(err)
	}

	body, err := json.Marshal(category)
	if err != nil {
		return utils.ServerError(err)
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Body:       string(body),
		Headers:    constants.CORS_HEADERS,
	}, nil
}

func CreateNewCategory(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	var createCategoryRequest models.CreateCategoryRequest
	err := json.Unmarshal([]byte(req.Body), &createCategoryRequest)
	if err != nil {
		log.Printf("Can't unmarshal body: %v", err)
		return utils.ClientError(http.StatusUnprocessableEntity)
	}
	validate := validator.New()
	err = validate.Struct(&createCategoryRequest)
	if err != nil {
		log.Printf("Invalid body: %v", err)
		return utils.ClientError(http.StatusBadRequest)
	}
	log.Printf("Received POST request with new category: %+v", createCategoryRequest)

	categoryDAO := persistence.NewCategoryDataAccessObject(&db)
	res, err := categoryDAO.InsertCategory(ctx, createCategoryRequest, req.RequestContext.Stage)
	if err != nil {
		return utils.ServerError(err)
	}
	log.Printf("Inserted new profile: %+v", res)

	json, err := json.Marshal(res)
	if err != nil {
		return utils.ServerError(err)
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusCreated,
		Body:       string(json),
		Headers:    constants.CORS_HEADERS,
	}, nil

}

func UpdateCategory(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	id, ok := req.QueryStringParameters["id"]
	if !ok {
		return utils.ClientError(http.StatusBadRequest)
	}
	var updateProfileRquest models.UpdateCategoryRequest
	err := json.Unmarshal([]byte(req.Body), &updateProfileRquest)
	if err != nil {
		log.Printf("Can't unmarshal body: %v", err)
		return utils.ClientError(http.StatusUnprocessableEntity)
	}
	log.Printf("Received PUT request with category: %+v", updateProfileRquest)

	categoryDAO := persistence.NewCategoryDataAccessObject(&db)
	res, err := categoryDAO.UpdateCategory(ctx, id, updateProfileRquest, req.RequestContext.Stage)
	if err != nil {
		return utils.ServerError(err)
	}

	if res == nil {
		return utils.ClientError(http.StatusNotFound)
	}

	log.Printf("Updated category: %+v", res)

	json, err := json.Marshal(res)
	if err != nil {
		return utils.ServerError(err)
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Body:       string(json),
		Headers:    constants.CORS_HEADERS,
	}, nil
}

func DeleteCategory(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	id, ok := req.QueryStringParameters["id"]
	if !ok {
		return utils.ClientError(http.StatusBadRequest)
	}
	log.Printf("Received DELETE request with id = %s", id)

	categoryDAO := persistence.NewCategoryDataAccessObject(&db)
	category, err := categoryDAO.DeleteCategory(ctx, id, req.RequestContext.Stage)
	if err != nil {
		return utils.ServerError(err)
	}

	if category == nil {
		return utils.ClientError(http.StatusNotFound)
	}

	json, err := json.Marshal(category)
	if err != nil {
		return utils.ServerError(err)
	}
	log.Printf("Successfully deleted category %+v", category)

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Body:       string(json),
		Headers:    constants.CORS_HEADERS,
	}, nil
}
