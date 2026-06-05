package api

import (
	"context"
	"encoding/json"
	"flashcard_lambda/internal/constants"
	"flashcard_lambda/internal/controllers"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	routeCategory          = "/category"
	routeDeck              = "/deck"
	routeTag               = "/tag"
	routeCardAnswerSection = "/card-answer-section"
	routeCardQuestionImage = "/card-question-image"
	routePresignedURL      = "/presigned-url"
)

func invalidRoute() events.APIGatewayProxyResponse {
	jbytes, err := json.Marshal(constants.DefaultResponseBody{Message: "Invalid route"})
	if err != nil {
		log.Fatal(err)
	}
	return events.APIGatewayProxyResponse{
		Body:       string(jbytes),
		StatusCode: 400,
		Headers:    constants.CORS_HEADERS,
	}
}

func ProcessGet(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client, s3Client s3.Client) (events.APIGatewayProxyResponse, error) {
	switch req.Path {
	case "/categories":
		return controllers.GetCategories(ctx, req, db)
	case routeCategory:
		return controllers.GetCategory(ctx, req, db)
	case "/decks":
		return controllers.GetDecks(ctx, req, db)
	case routeDeck:
		return controllers.GetDeck(ctx, req, db)
	case "/tags":
		return controllers.GetTags(ctx, req, db)
	case routeTag:
		return controllers.GetTag(ctx, req, db)
	case "/card-answer-sections":
		return controllers.GetCardAnswerSections(ctx, req, db)
	case routeCardAnswerSection:
		return controllers.GetCardAnswerSection(ctx, req, db)
	case "/card-question-images":
		return controllers.GetCardQuestionImages(ctx, req, db)
	case routeCardQuestionImage:
		return controllers.GetCardQuestionImage(ctx, req, db)
	case routePresignedURL:
		return controllers.GetPresignedURL(ctx, req, s3Client)
	default:
		return invalidRoute(), nil
	}
}

func ProcessPost(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	switch req.Path {
	case routeCategory:
		return controllers.CreateNewCategory(ctx, req, db)
	case routeDeck:
		return controllers.CreateNewDeck(ctx, req, db)
	case routeTag:
		return controllers.CreateNewTag(ctx, req, db)
	case routeCardAnswerSection:
		return controllers.CreateNewCardAnswerSection(ctx, req, db)
	case routeCardQuestionImage:
		return controllers.CreateNewCardQuestionImage(ctx, req, db)
	default:
		return invalidRoute(), nil
	}
}

func ProcessPut(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	switch req.Path {
	case routeCategory:
		return controllers.UpdateCategory(ctx, req, db)
	case routeDeck:
		return controllers.UpdateDeck(ctx, req, db)
	case routeTag:
		return controllers.UpdateTag(ctx, req, db)
	case routeCardAnswerSection:
		return controllers.UpdateCardAnswerSection(ctx, req, db)
	case routeCardQuestionImage:
		return controllers.UpdateCardQuestionImage(ctx, req, db)
	default:
		return invalidRoute(), nil
	}
}

func ProcessDelete(ctx context.Context, req events.APIGatewayProxyRequest, db dynamodb.Client) (events.APIGatewayProxyResponse, error) {
	switch req.Path {
	case routeCategory:
		return controllers.DeleteCategory(ctx, req, db)
	case routeDeck:
		return controllers.DeleteDeck(ctx, req, db)
	case routeTag:
		return controllers.DeleteTag(ctx, req, db)
	case routeCardAnswerSection:
		return controllers.DeleteCardAnswerSection(ctx, req, db)
	case routeCardQuestionImage:
		return controllers.DeleteCardQuestionImage(ctx, req, db)
	default:
		return invalidRoute(), nil
	}
}
