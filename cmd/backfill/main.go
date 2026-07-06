// One-time migration: sets entity_type on legacy items that predate it.
// Only card_answer_section and card_question_image strictly need it (they
// share card_id-index and are disambiguated by an entity_type filter), but
// decks and cards are backfilled too for consistency.
//
// Usage:
//
//	go run ./cmd/backfill -table flash-card-app-dev            # dry run
//	go run ./cmd/backfill -table flash-card-app-dev -apply
package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"flashcard_lambda/internal/models"
)

func inferEntityType(item map[string]dynamodbTypes.AttributeValue) string {
	has := func(attr string) bool {
		_, ok := item[attr]
		return ok
	}
	switch {
	case has("category_id"):
		return models.EntityTypeDeck
	case has("deck_id"):
		return models.EntityTypeCard
	case has("card_answer_section_id"):
		return models.EntityTypeCardAnswerSectionImage
	case has("card_id") && has("image_url"):
		return models.EntityTypeCardQuestionImage
	case has("card_id"):
		return models.EntityTypeCardAnswerSection
	default:
		// Categories and tags without entity_type can't be told apart;
		// they must be fixed by hand (listing already requires the field).
		return ""
	}
}

func main() {
	table := flag.String("table", "", "DynamoDB table name (required)")
	apply := flag.Bool("apply", false, "write changes (default is dry run)")
	flag.Parse()
	if *table == "" {
		log.Fatal("-table is required")
	}

	ctx := context.Background()
	sdkConfig, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatal(err)
	}
	db := dynamodb.NewFromConfig(sdkConfig)

	input := &dynamodb.ScanInput{TableName: table}
	updated, skipped, unknown := 0, 0, 0
	for {
		res, err := db.Scan(ctx, input)
		if err != nil {
			log.Fatal(err)
		}

		for _, item := range res.Items {
			if _, ok := item["entity_type"]; ok {
				skipped++
				continue
			}

			entityType := inferEntityType(item)
			id := ""
			if idAttr, ok := item["id"].(*dynamodbTypes.AttributeValueMemberS); ok {
				id = idAttr.Value
			}
			if entityType == "" || id == "" {
				unknown++
				fmt.Printf("SKIP (cannot infer): %v\n", item["id"])
				continue
			}

			fmt.Printf("%s -> entity_type=%s\n", id, entityType)
			if *apply {
				_, err := db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
					TableName:                 table,
					Key:                       map[string]dynamodbTypes.AttributeValue{"id": &dynamodbTypes.AttributeValueMemberS{Value: id}},
					UpdateExpression:          aws.String("SET entity_type = :t"),
					ExpressionAttributeValues: map[string]dynamodbTypes.AttributeValue{":t": &dynamodbTypes.AttributeValueMemberS{Value: entityType}},
				})
				if err != nil {
					log.Fatalf("updating %s: %v", id, err)
				}
			}
			updated++
		}

		if res.LastEvaluatedKey == nil {
			break
		}
		input.ExclusiveStartKey = res.LastEvaluatedKey
	}

	mode := "DRY RUN (use -apply to write)"
	if *apply {
		mode = "APPLIED"
	}
	fmt.Printf("\n%s: %d to update, %d already set, %d could not infer\n", mode, updated, skipped, unknown)
}
