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

func GetCardAnswerSections(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	cardId, ok := req.QueryStringParameters["cardId"]
	if !ok {
		return utils.ClientError(http.StatusBadRequest)
	}
	log.Printf("Received GET card answer sections request for cardId = %s", cardId)

	dao := persistence.NewCardAnswerSectionDataAccessObject(&db)
	sections, err := dao.GetCardAnswerSections(ctx, cardId, req.RequestContext.Stage)
	if err != nil {
		return utils.ServerError(err)
	}
	log.Printf("Retrieved %d card answer sections", len(sections))

	body, err := json.Marshal(sections)
	if err != nil {
		return utils.ServerError(err)
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Body:       string(body),
		Headers:    constants.CORS_HEADERS,
	}, nil
}

func GetCardAnswerSection(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	id, ok := req.QueryStringParameters["id"]
	if !ok {
		return utils.ClientError(http.StatusBadRequest)
	}
	log.Printf("Received GET card answer section request with id = %s", id)

	dao := persistence.NewCardAnswerSectionDataAccessObject(&db)
	section, err := dao.GetCardAnswerSection(ctx, id, req.RequestContext.Stage)
	if err != nil {
		return utils.ServerError(err)
	}

	body, err := json.Marshal(section)
	if err != nil {
		return utils.ServerError(err)
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Body:       string(body),
		Headers:    constants.CORS_HEADERS,
	}, nil
}

func CreateNewCardAnswerSection(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	var createReq models.CreateCardAnswerSectionRequest
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
	log.Printf("Received POST request with new card answer section: %+v", createReq)

	dao := persistence.NewCardAnswerSectionDataAccessObject(&db)
	res, err := dao.InsertCardAnswerSection(ctx, createReq, req.RequestContext.Stage)
	if err != nil {
		return utils.ServerError(err)
	}
	log.Printf("Inserted new card answer section: %+v", res)

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

func UpdateCardAnswerSection(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	id, ok := req.QueryStringParameters["id"]
	if !ok {
		return utils.ClientError(http.StatusBadRequest)
	}

	var updateReq models.UpdateCardAnswerSectionRequest
	err := json.Unmarshal([]byte(req.Body), &updateReq)
	if err != nil {
		log.Printf("Can't unmarshal body: %v", err)
		return utils.ClientError(http.StatusUnprocessableEntity)
	}
	log.Printf("Received PUT request with card answer section: %+v", updateReq)

	dao := persistence.NewCardAnswerSectionDataAccessObject(&db)
	res, err := dao.UpdateCardAnswerSection(ctx, id, updateReq, req.RequestContext.Stage)
	if err != nil {
		return utils.ServerError(err)
	}

	if res == nil {
		return utils.ClientError(http.StatusNotFound)
	}

	log.Printf("Updated card answer section: %+v", res)

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

func DeleteCardAnswerSection(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	id, ok := req.QueryStringParameters["id"]
	if !ok {
		return utils.ClientError(http.StatusBadRequest)
	}
	log.Printf("Received DELETE request with id = %s", id)

	dao := persistence.NewCardAnswerSectionDataAccessObject(&db)
	section, err := dao.DeleteCardAnswerSection(ctx, id, req.RequestContext.Stage)
	if err != nil {
		return utils.ServerError(err)
	}

	if section == nil {
		return utils.ClientError(http.StatusNotFound)
	}

	body, err := json.Marshal(section)
	if err != nil {
		return utils.ServerError(err)
	}
	log.Printf("Successfully deleted card answer section %+v", section)

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Body:       string(body),
		Headers:    constants.CORS_HEADERS,
	}, nil
}
