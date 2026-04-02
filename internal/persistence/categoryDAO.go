package persistence

import (
	"context"
	"errors"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go"
	"github.com/google/uuid"

	"flashcard_lambda/internal/constants"
	"flashcard_lambda/internal/models"
)

type ICategoryDataAccessObject interface {
	GetCategories(ctx context.Context, stage string) ([]models.Category, error)
	GetCategory(ctx context.Context, id string, stage string) (*models.Category, error)
	InsertCategory(ctx context.Context, createCategory models.CreateCategoryRequest, stage string) (*models.Category, error)
	UpdateCategory(ctx context.Context, id string, updateCategory models.UpdateCategoryRequest, stage string) (*models.Category, error)
	DeleteCategory(ctx context.Context, id string, stage string) (*models.Category, error)
}

type CategoryDataAccessObject struct {
	db *dynamodb.Client
}

func NewCategoryDataAccessObject(db *dynamodb.Client) ICategoryDataAccessObject {
	return &CategoryDataAccessObject{
		db: db,
	}
}

const filterCategories = "attribute_not_exists(set_id) AND attribute_not_exists(category_id)"

func (categoryDAO *CategoryDataAccessObject) GetCategories(ctx context.Context, stage string) ([]models.Category, error) {
	input := &dynamodb.ScanInput{
		TableName:        aws.String(constants.GetDBName(stage)),
		FilterExpression: aws.String(filterCategories),
	}

	var categories []models.Category
	for {
		res, err := categoryDAO.db.Scan(ctx, input)
		if err != nil {
			return nil, err
		}

		var page []models.Category
		if err = attributevalue.UnmarshalListOfMaps(res.Items, &page); err != nil {
			return nil, err
		}
		categories = append(categories, page...)

		if res.LastEvaluatedKey == nil {
			break
		}
		input.ExclusiveStartKey = res.LastEvaluatedKey
	}

	return categories, nil
}

func (categoryDAO *CategoryDataAccessObject) GetCategory(ctx context.Context, id string, stage string) (*models.Category, error) {
	key, err := attributevalue.Marshal(id)
	if err != nil {
		return nil, err
	}

	input := &dynamodb.GetItemInput{
		TableName: aws.String(constants.GetDBName(stage)),
		Key: map[string]dynamodbTypes.AttributeValue{
			"id": key,
		},
	}

	log.Printf("Calling Dynamodb with input: %v", input)
	result, err := categoryDAO.db.GetItem(ctx, input)
	if err != nil {
		return nil, err
	}
	log.Printf("Executed GetItem DynamoDb successfully. Result: %#v", result)

	if result.Item == nil {
		return nil, nil
	}

	category := new(models.Category)
	err = attributevalue.UnmarshalMap(result.Item, category)
	if err != nil {
		return nil, err
	}

	return category, nil
}

func (categoryDAO *CategoryDataAccessObject) InsertCategory(ctx context.Context, createCategory models.CreateCategoryRequest, stage string) (*models.Category, error) {
	category := models.Category{
		Id:          uuid.NewString(),
		Name:        createCategory.Name,
		Description: createCategory.Description,
	}

	item, err := attributevalue.MarshalMap(category)
	if err != nil {
		return nil, err
	}
	input := &dynamodb.PutItemInput{
		TableName: aws.String(constants.GetDBName(stage)),
		Item:      item,
	}
	res, err := categoryDAO.db.PutItem(ctx, input)
	if err != nil {
		return nil, err
	}

	err = attributevalue.UnmarshalMap(res.Attributes, &category)
	if err != nil {
		return nil, err
	}

	return &category, nil
}

func (categoryDAO *CategoryDataAccessObject) UpdateCategory(ctx context.Context, id string, updateCategory models.UpdateCategoryRequest, stage string) (*models.Category, error) {
	key, err := attributevalue.Marshal(id)
	if err != nil {
		return nil, err
	}
	expr, err := expression.NewBuilder().WithUpdate(
		expression.Set(
			expression.Name("name"),
			expression.Value(updateCategory.Name),
		).Set(
			expression.Name("description"),
			expression.Value(updateCategory.Description),
		),
	).WithCondition(
		expression.Equal(
			expression.Name("id"),
			expression.Value(id),
		),
	).Build()
	if err != nil {
		return nil, err
	}

	input := &dynamodb.UpdateItemInput{
		Key: map[string]dynamodbTypes.AttributeValue{
			"id": key,
		},
		TableName:                 aws.String(constants.GetDBName(stage)),
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		ConditionExpression:       expr.Condition(),
		ReturnValues:              dynamodbTypes.ReturnValue(*aws.String("ALL_NEW")),
	}

	res, err := categoryDAO.db.UpdateItem(ctx, input)
	if err != nil {
		var smErr *smithy.OperationError
		if errors.As(err, &smErr) {
			var condCheckFailed *dynamodbTypes.ConditionalCheckFailedException
			if errors.As(err, &condCheckFailed) {
				return nil, nil
			}
		}

		return nil, err
	}

	if res.Attributes == nil {
		return nil, nil
	}
	category := new(models.Category)
	err = attributevalue.UnmarshalMap(res.Attributes, category)
	if err != nil {
		return nil, err
	}

	return category, nil
}

func (categoryDAO *CategoryDataAccessObject) DeleteCategory(ctx context.Context, id string, stage string) (*models.Category, error) {
	key, err := attributevalue.Marshal(id)
	if err != nil {
		return nil, err
	}

	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(constants.GetDBName(stage)),
		Key: map[string]dynamodbTypes.AttributeValue{
			"id": key,
		},
		ReturnValues: dynamodbTypes.ReturnValue(*aws.String("ALL_OLD")),
	}

	res, err := categoryDAO.db.DeleteItem(ctx, input)
	if err != nil {
		return nil, err
	}

	if res.Attributes == nil {
		return nil, nil
	}

	category := new(models.Category)
	err = attributevalue.UnmarshalMap(res.Attributes, category)
	if err != nil {
		return nil, err
	}

	return category, nil
}
