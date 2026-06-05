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
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var db dynamodb.Client
var s3Client s3.Client

func init() {
	sdkConfig, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatal(err)
	}

	db = *dynamodb.NewFromConfig(sdkConfig)
	s3Client = *s3.NewFromConfig(sdkConfig)
}

func main() {
	lambda.Start(HandleRequest)
}

func HandleRequest(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	log.Printf("Received req %#v", req)
	log.Printf("Stage: %#v", req.RequestContext.Stage)
	switch req.HTTPMethod {
	case "GET":
		return api.ProcessGet(ctx, req, db, s3Client)
	case "POST":
		return api.ProcessPost(ctx, req, db)
	case "PUT":
		return api.ProcessPut(ctx, req, db)
	case "DELETE":
		return api.ProcessDelete(ctx, req, db, s3Client)
	default:
		return utils.ClientError(http.StatusMethodNotAllowed)
	}
}
