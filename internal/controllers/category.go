package controllers

import (
	"context"
	"encoding/json"
	"flashcard_lambda/internal/constants"
	"flashcard_lambda/internal/models"
	"flashcard_lambda/internal/utils"
	"log"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/google/uuid"
	"gopkg.in/go-playground/validator.v9"
)

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

	isDev := req.RequestContext.Stage == "dev"
	res, err := insertCategory(ctx, createCategoryRequest, db, isDev)
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

func insertCategory(ctx context.Context, createCategory models.CreateCategoryRequest, db dynamodb.Client, isDev bool) (*models.Category, error) {
	category := models.Category{
		Id:          uuid.NewString(),
		Name:        createCategory.Name,
		Description: createCategory.Description,
	}

	item, err := attributevalue.MarshalMap(category)
	if err != nil {
		return nil, err
	}
	dbName := constants.DYNAMODB_NAME
	if isDev {
		dbName = constants.DYNAMODB_NAME_DEV
	}
	input := &dynamodb.PutItemInput{
		TableName: aws.String(dbName),
		Item:      item,
	}
	res, err := db.PutItem(ctx, input)
	if err != nil {
		return nil, err
	}

	err = attributevalue.UnmarshalMap(res.Attributes, &category)
	if err != nil {
		return nil, err
	}

	return &category, nil
}
