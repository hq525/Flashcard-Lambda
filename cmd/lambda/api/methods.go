package api

import (
	"context"
	"encoding/json"
	"flashcard_lambda/internal/constants"
	"flashcard_lambda/internal/controllers"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

func ProcessGet(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	ApiResponse := events.APIGatewayProxyResponse{}
	path := req.Path
	var err error
	switch path {
	default:
		responseBody := constants.DefaultResponseBody{
			Message: "Invalid route",
		}
		jbytes, err := json.Marshal(responseBody)
		if err != nil {
			log.Fatal(err)
		}
		jstr := string(jbytes)
		ApiResponse = events.APIGatewayProxyResponse{Body: jstr, StatusCode: 400}
	}

	if err != nil {
		responseBody := constants.DefaultResponseBody{
			Message: err.Error(),
		}
		jbytes, _ := json.Marshal(responseBody)
		jstr := string(jbytes)
		ApiResponse = events.APIGatewayProxyResponse{Body: jstr, StatusCode: 400}
	}
	var headers = constants.CORS_HEADERS

	corsApiResponse := events.APIGatewayProxyResponse{
		Body:       ApiResponse.Body,
		StatusCode: ApiResponse.StatusCode,
		Headers:    headers,
	}
	return corsApiResponse, nil
}

func ProcessPost(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	ApiResponse := events.APIGatewayProxyResponse{}
	path := req.Path
	var err error
	switch path {
	case "/category":
		return controllers.CreateNewCategory(ctx, req, db)
	default:
		responseBody := constants.DefaultResponseBody{
			Message: "Invalid route",
		}
		jbytes, err := json.Marshal(responseBody)
		if err != nil {
			log.Fatal(err)
		}
		jstr := string(jbytes)
		ApiResponse = events.APIGatewayProxyResponse{Body: jstr, StatusCode: 400}
	}

	if err != nil {
		responseBody := constants.DefaultResponseBody{
			Message: err.Error(),
		}
		jbytes, _ := json.Marshal(responseBody)
		jstr := string(jbytes)
		ApiResponse = events.APIGatewayProxyResponse{Body: jstr, StatusCode: 400}
	}
	var headers = constants.CORS_HEADERS

	corsApiResponse := events.APIGatewayProxyResponse{
		Body:       ApiResponse.Body,
		StatusCode: ApiResponse.StatusCode,
		Headers:    headers,
	}
	return corsApiResponse, nil
}
