// Package app wires the concrete implementations (DynamoDB, S3) into the
// http.Handler. This is the only place implementations are chosen; both
// the Lambda and local-server entry points call NewHandler.
package app

import (
	"context"
	"net/http"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"flashcard_lambda/internal/config"
	"flashcard_lambda/internal/httpapi"
	"flashcard_lambda/internal/persistence"
	"flashcard_lambda/internal/service"
	"flashcard_lambda/internal/storage"
)

func NewHandler(ctx context.Context) (http.Handler, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	sdkConfig, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	store := &persistence.Store{
		DB:    dynamodb.NewFromConfig(sdkConfig),
		Table: cfg.TableName,
	}
	images := storage.NewS3ImageStore(s3.NewFromConfig(sdkConfig), cfg.Bucket)

	categories := persistence.NewCategoryRepository(store)
	decks := persistence.NewDeckRepository(store)
	tags := persistence.NewTagRepository(store)
	cards := persistence.NewCardRepository(store)
	sections := persistence.NewCardAnswerSectionRepository(store)
	questionImages := persistence.NewCardQuestionImageRepository(store)
	sectionImages := persistence.NewCardAnswerSectionImageRepository(store)

	cascade := &service.Cascade{
		Categories:     categories,
		Decks:          decks,
		Cards:          cards,
		Sections:       sections,
		QuestionImages: questionImages,
		SectionImages:  sectionImages,
		Images:         images,
	}

	return httpapi.NewRouter(httpapi.Deps{
		Categories:     categories,
		Decks:          decks,
		Tags:           tags,
		Cards:          cards,
		Sections:       sections,
		QuestionImages: questionImages,
		SectionImages:  sectionImages,
		Images:         images,
		Cascade:        cascade,
	}), nil
}
