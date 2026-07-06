package main

import (
	"context"
	"log"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	"flashcard_lambda/internal/app"
)

func main() {
	handler, err := app.NewHandler(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	lambda.Start(httpadapter.New(handler).ProxyWithContext)
}
