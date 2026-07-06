package persistence

import (
	"context"
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

func GetItem[T any](ctx context.Context, s *Store, id string) (*T, error) {
	key, err := idKey(id)
	if err != nil {
		return nil, err
	}

	result, err := s.DB.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.Table),
		Key:       key,
	})
	if err != nil {
		return nil, err
	}
	if result.Item == nil {
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

// UpdateItem sets the given attributes on the item with the given id and
// returns the updated item, or nil if the item does not exist.
func UpdateItem[T any](ctx context.Context, s *Store, id string, attrs map[string]any) (*T, error) {
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
		WithCondition(expression.AttributeExists(expression.Name("id"))).
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

// DeleteItem removes the item with the given id and returns the deleted
// item, or nil if the item did not exist.
func DeleteItem[T any](ctx context.Context, s *Store, id string) (*T, error) {
	key, err := idKey(id)
	if err != nil {
		return nil, err
	}

	res, err := s.DB.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName:    aws.String(s.Table),
		Key:          key,
		ReturnValues: dynamodbTypes.ReturnValueAllOld,
	})
	if err != nil {
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

	var items []T
	for {
		res, err := s.DB.Query(ctx, input)
		if err != nil {
			return nil, err
		}

		var page []T
		if err = attributevalue.UnmarshalListOfMaps(res.Items, &page); err != nil {
			return nil, err
		}
		items = append(items, page...)

		if res.LastEvaluatedKey == nil {
			break
		}
		input.ExclusiveStartKey = res.LastEvaluatedKey
	}

	return items, nil
}
