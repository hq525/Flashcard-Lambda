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

func GetDecks(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	categoryId, ok := req.QueryStringParameters["categoryId"]
	if !ok {
		return utils.ClientError(http.StatusBadRequest)
	}
	log.Printf("Received GET decks request for categoryId = %s", categoryId)

	deckDAO := persistence.NewDeckDataAccessObject(&db)
	decks, err := deckDAO.GetDecks(ctx, categoryId, req.RequestContext.Stage)
	if err != nil {
		return utils.ServerError(err)
	}
	log.Printf("Retrieved %d decks", len(decks))

	body, err := json.Marshal(decks)
	if err != nil {
		return utils.ServerError(err)
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Body:       string(body),
		Headers:    constants.CORS_HEADERS,
	}, nil
}

func GetDeck(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	id, ok := req.QueryStringParameters["id"]
	if !ok {
		return utils.ClientError(http.StatusBadRequest)
	}
	log.Printf("Received GET deck request with id = %s", id)

	deckDAO := persistence.NewDeckDataAccessObject(&db)
	deck, err := deckDAO.GetDeck(ctx, id, req.RequestContext.Stage)
	if err != nil {
		return utils.ServerError(err)
	}

	body, err := json.Marshal(deck)
	if err != nil {
		return utils.ServerError(err)
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Body:       string(body),
		Headers:    constants.CORS_HEADERS,
	}, nil
}

func CreateNewDeck(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	var createDeckRequest models.CreateDeckRequest
	err := json.Unmarshal([]byte(req.Body), &createDeckRequest)
	if err != nil {
		log.Printf("Can't unmarshal body: %v", err)
		return utils.ClientError(http.StatusUnprocessableEntity)
	}

	err = validate.Struct(&createDeckRequest)
	if err != nil {
		log.Printf("Invalid body: %v", err)
		return utils.ClientError(http.StatusBadRequest)
	}
	log.Printf("Received POST request with new deck: %+v", createDeckRequest)

	deckDAO := persistence.NewDeckDataAccessObject(&db)
	res, err := deckDAO.InsertDeck(ctx, createDeckRequest, req.RequestContext.Stage)
	if err != nil {
		return utils.ServerError(err)
	}
	log.Printf("Inserted new deck: %+v", res)

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

func UpdateDeck(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	id, ok := req.QueryStringParameters["id"]
	if !ok {
		return utils.ClientError(http.StatusBadRequest)
	}

	var updateDeckRequest models.UpdateDeckRequest
	err := json.Unmarshal([]byte(req.Body), &updateDeckRequest)
	if err != nil {
		log.Printf("Can't unmarshal body: %v", err)
		return utils.ClientError(http.StatusUnprocessableEntity)
	}
	log.Printf("Received PUT request with deck: %+v", updateDeckRequest)

	deckDAO := persistence.NewDeckDataAccessObject(&db)
	res, err := deckDAO.UpdateDeck(ctx, id, updateDeckRequest, req.RequestContext.Stage)
	if err != nil {
		return utils.ServerError(err)
	}

	if res == nil {
		return utils.ClientError(http.StatusNotFound)
	}

	log.Printf("Updated deck: %+v", res)

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

func DeleteDeck(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	id, ok := req.QueryStringParameters["id"]
	if !ok {
		return utils.ClientError(http.StatusBadRequest)
	}
	log.Printf("Received DELETE request with id = %s", id)

	deckDAO := persistence.NewDeckDataAccessObject(&db)
	deck, err := deckDAO.DeleteDeck(ctx, id, req.RequestContext.Stage)
	if err != nil {
		return utils.ServerError(err)
	}

	if deck == nil {
		return utils.ClientError(http.StatusNotFound)
	}

	body, err := json.Marshal(deck)
	if err != nil {
		return utils.ServerError(err)
	}
	log.Printf("Successfully deleted deck %+v", deck)

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Body:       string(body),
		Headers:    constants.CORS_HEADERS,
	}, nil
}
