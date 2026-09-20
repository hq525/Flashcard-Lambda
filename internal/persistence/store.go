package persistence

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Store wraps a DynamoDB client and table name. All entities live in one
// table keyed by "id"; list operations query GSIs (see entities.go).
type Store struct {
	DB    *dynamodb.Client
	Table string
}

func idKey(id string) (map[string]dynamodbTypes.AttributeValue, error) {
	key, err := attributevalue.Marshal(id)
	if err != nil {
		return nil, err
	}
	return map[string]dynamodbTypes.AttributeValue{"id": key}, nil
}

func GetItem[T any](ctx context.Context, s *Store, id, entityType string) (*T, error) {
	key, err := idKey(id)
	if err != nil {
		return nil, err
	}

	result, err := s.DB.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(s.Table),
		Key:            key,
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, err
	}
	storedType, ok := result.Item["entity_type"].(*dynamodbTypes.AttributeValueMemberS)
	if !ok || storedType.Value != entityType {
		return nil, nil
	}

	item := new(T)
	if err = attributevalue.UnmarshalMap(result.Item, item); err != nil {
		return nil, err
	}
	return item, nil
}

func PutItem(ctx context.Context, s *Store, entity any) error {
	item, err := attributevalue.MarshalMap(entity)
	if err != nil {
		return err
	}

	_, err = s.DB.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.Table),
		Item:      item,
	})
	return err
}

// UpdateItem sets attributes only on an existing item of the required entity
// type. Missing items and IDs belonging to another entity type return nil.
func UpdateItem[T any](ctx context.Context, s *Store, id, entityType string, attrs map[string]any) (*T, error) {
	key, err := idKey(id)
	if err != nil {
		return nil, err
	}

	var update expression.UpdateBuilder
	for name, value := range attrs {
		update = update.Set(expression.Name(name), expression.Value(value))
	}
	expr, err := expression.NewBuilder().
		WithUpdate(update).
		WithCondition(expression.And(
			expression.AttributeExists(expression.Name("id")),
			expression.Name("entity_type").Equal(expression.Value(entityType)),
		)).
		Build()
	if err != nil {
		return nil, err
	}

	res, err := s.DB.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 aws.String(s.Table),
		Key:                       key,
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		ConditionExpression:       expr.Condition(),
		ReturnValues:              dynamodbTypes.ReturnValueAllNew,
	})
	if err != nil {
		var condCheckFailed *dynamodbTypes.ConditionalCheckFailedException
		if errors.As(err, &condCheckFailed) {
			return nil, nil
		}
		return nil, err
	}

	item := new(T)
	if err = attributevalue.UnmarshalMap(res.Attributes, item); err != nil {
		return nil, err
	}
	return item, nil
}

// DeleteItem removes an existing item only when its entity type matches and
// returns it. Missing items and IDs belonging to another entity type return nil.
func DeleteItem[T any](ctx context.Context, s *Store, id, entityType string) (*T, error) {
	key, err := idKey(id)
	if err != nil {
		return nil, err
	}

	expr, err := expression.NewBuilder().WithCondition(expression.And(
		expression.AttributeExists(expression.Name("id")),
		expression.Name("entity_type").Equal(expression.Value(entityType)),
	)).Build()
	if err != nil {
		return nil, err
	}

	res, err := s.DB.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName:                 aws.String(s.Table),
		Key:                       key,
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		ReturnValues:              dynamodbTypes.ReturnValueAllOld,
	})
	if err != nil {
		var condCheckFailed *dynamodbTypes.ConditionalCheckFailedException
		if errors.As(err, &condCheckFailed) {
			return nil, nil
		}
		return nil, err
	}
	if res.Attributes == nil {
		return nil, nil
	}

	item := new(T)
	if err = attributevalue.UnmarshalMap(res.Attributes, item); err != nil {
		return nil, err
	}
	return item, nil
}

// QueryIndex queries a GSI for all items whose partition key attribute
// equals keyValue, paginating through all results. If entityTypeFilter is
// non-empty, results are additionally filtered on entity_type (needed when
// two entity types share an index key, e.g. card_id).
func QueryIndex[T any](ctx context.Context, s *Store, index, keyAttr, keyValue, entityTypeFilter string) ([]T, error) {
	builder := expression.NewBuilder().WithKeyCondition(
		expression.Key(keyAttr).Equal(expression.Value(keyValue)),
	)
	if entityTypeFilter != "" {
		builder = builder.WithFilter(expression.Equal(
			expression.Name("entity_type"),
			expression.Value(entityTypeFilter),
		))
	}
	expr, err := builder.Build()
	if err != nil {
		return nil, err
	}

	input := &dynamodb.QueryInput{
		TableName:                 aws.String(s.Table),
		IndexName:                 aws.String(index),
		KeyConditionExpression:    expr.KeyCondition(),
		FilterExpression:          expr.Filter(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	}

	return queryBounded[T](ctx, s, input)
}

const (
	maxResultItems    = 1000
	maxQueryPages     = 50
	maxQueryPageItems = 100
	// MaxResultBytes bounds collected records. The HTTP layer separately checks
	// the escaped API Gateway/Lambda response envelope before writing success.
	MaxResultBytes = 4 * 1024 * 1024
)

// ErrResultLimit indicates that a complete result cannot be returned within the
// request budget. Callers must not treat it as an empty or partial list.
var ErrResultLimit = errors.New("result exceeds the supported item, page, or byte limit")

func queryBounded[T any](ctx context.Context, s *Store, input *dynamodb.QueryInput) ([]T, error) {
	items := make([]T, 0)
	resultBytes := 2 // JSON array brackets.
	for pageNumber := 0; pageNumber < maxQueryPages; pageNumber++ {
		// A single extra item proves that the complete list exceeds the cap.
		input.Limit = aws.Int32(int32(min(maxQueryPageItems, maxResultItems-len(items)+1)))
		res, err := s.DB.Query(ctx, input)
		if err != nil {
			return nil, err
		}

		var page []T
		if err = attributevalue.UnmarshalListOfMaps(res.Items, &page); err != nil {
			return nil, err
		}
		if len(items)+len(page) > maxResultItems {
			return nil, ErrResultLimit
		}
		for _, item := range page {
			encoded, err := json.Marshal(item)
			if err != nil {
				return nil, err
			}
			resultBytes += len(encoded) + 1 // Include a separator (conservative for the first item).
			if resultBytes > MaxResultBytes {
				return nil, ErrResultLimit
			}
		}
		items = append(items, page...)

		if len(res.LastEvaluatedKey) == 0 {
			return items, nil
		}
		input.ExclusiveStartKey = res.LastEvaluatedKey
	}

	return nil, ErrResultLimit
}
