package lambda

import (
	"context"
	"flashcard_lambda/cmd/lambda/api"
	"flashcard_lambda/internal/utils"
	"log"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

var db dynamodb.Client

func init() {
	sdkConfig, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatal(err)
	}

	db = *dynamodb.NewFromConfig(sdkConfig)
}

func main() {
	lambda.Start(HandleRequest)
}

func HandleRequest(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	log.Printf("Received req %#v", req)
	log.Printf("Stage: %#v", req.RequestContext.Stage)
	switch req.HTTPMethod {
	case "GET":
		return api.ProcessGet(ctx, req, db)
	default:
		return utils.ClientError(http.StatusMethodNotAllowed)
	}
}
