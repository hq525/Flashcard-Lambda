package persistence

import (
	"context"
	"errors"

	"flashcard_lambda/internal/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// cardRepository retains generic CRUD behavior except for edits, whose allowed
// attributes depend on whether a first FSRS review has migrated the card.
type cardRepository struct {
	*DynamoRepository[models.Card, models.CreateCardRequest, models.UpdateCardRequest]
}

func (r *cardRepository) Update(ctx context.Context, id string, req models.UpdateCardRequest) (*models.Card, error) {
	legacyAttrs := r.cfg.UpdateAttrs(req)
	contentAttrs := map[string]any{
		"question": legacyAttrs["question"], "tag_ids": legacyAttrs["tag_ids"],
		"updated_date_time": legacyAttrs["updated_date_time"],
	}
	// Schedule creation is monotonic: a card moves from legacy to FSRS once.
	// Try the common migrated case first. A legacy update is allowed only if
	// schedule remains absent at write time. If a first review races these
	// attempts, one migrated retry completes the edit without overwriting it.
	for _, migrated := range []bool{true, false, true} {
		attrs := contentAttrs
		if !migrated {
			attrs = legacyAttrs
		}
		card, conditionFailed, err := r.updateCard(ctx, id, attrs, migrated)
		if err != nil || !conditionFailed {
			return card, err
		}
	}
	// All attempts require an existing card. With monotonic migration, the
	// final failure means the card does not exist (or is a different entity).
	return nil, nil
}

func (r *cardRepository) updateCard(ctx context.Context, id string, attrs map[string]any, migrated bool) (*models.Card, bool, error) {
	key, err := idKey(id)
	if err != nil {
		return nil, false, err
	}
	var update expression.UpdateBuilder
	for name, value := range attrs {
		update = update.Set(expression.Name(name), expression.Value(value))
	}
	scheduleCondition := expression.AttributeNotExists(expression.Name("schedule"))
	if migrated {
		scheduleCondition = expression.AttributeExists(expression.Name("schedule"))
	}
	condition := expression.And(
		expression.AttributeExists(expression.Name("id")),
		expression.Name("entity_type").Equal(expression.Value(models.EntityTypeCard)),
		scheduleCondition,
	)
	expr, err := expression.NewBuilder().WithUpdate(update).WithCondition(condition).Build()
	if err != nil {
		return nil, false, err
	}
	result, err := r.store.DB.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.store.Table), Key: key,
		UpdateExpression: expr.Update(), ConditionExpression: expr.Condition(),
		ExpressionAttributeNames: expr.Names(), ExpressionAttributeValues: expr.Values(),
		ReturnValues: dynamodbTypes.ReturnValueAllNew,
	})
	if err != nil {
		var conditionFailed *dynamodbTypes.ConditionalCheckFailedException
		if errors.As(err, &conditionFailed) {
			return nil, true, nil
		}
		return nil, false, err
	}
	card := new(models.Card)
	if err := attributevalue.UnmarshalMap(result.Attributes, card); err != nil {
		return nil, false, err
	}
	return card, false, nil
}
