package persistence

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"

	"flashcard_lambda/internal/constants"
	"flashcard_lambda/internal/models"
)

type ITagDataAccessObject interface {
	GetTags(ctx context.Context, stage string) ([]models.Tag, error)
	GetTag(ctx context.Context, id string, stage string) (*models.Tag, error)
	InsertTag(ctx context.Context, createTag models.CreateTagRequest, stage string) (*models.Tag, error)
	UpdateTag(ctx context.Context, id string, updateTag models.UpdateTagRequest, stage string) (*models.Tag, error)
	DeleteTag(ctx context.Context, id string, stage string) (*models.Tag, error)
}

type TagDataAccessObject struct {
	db *dynamodb.Client
}

func NewTagDataAccessObject(db *dynamodb.Client) ITagDataAccessObject {
	return &TagDataAccessObject{
		db: db,
	}
}

func (tagDAO *TagDataAccessObject) GetTags(ctx context.Context, stage string) ([]models.Tag, error) {
	input := &dynamodb.ScanInput{
		TableName: aws.String(constants.GetDBName(stage)),
	}

	var tags []models.Tag
	for {
		res, err := tagDAO.db.Scan(ctx, input)
		if err != nil {
			return nil, err
		}

		var page []models.Tag
		if err = attributevalue.UnmarshalListOfMaps(res.Items, &page); err != nil {
			return nil, err
		}
		tags = append(tags, page...)

		if res.LastEvaluatedKey == nil {
			break
		}
		input.ExclusiveStartKey = res.LastEvaluatedKey
	}

	return tags, nil
}

func (tagDAO *TagDataAccessObject) GetTag(ctx context.Context, id string, stage string) (*models.Tag, error) {
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

	result, err := tagDAO.db.GetItem(ctx, input)
	if err != nil {
		return nil, err
	}

	if result.Item == nil {
		return nil, nil
	}

	tag := new(models.Tag)
	err = attributevalue.UnmarshalMap(result.Item, tag)
	if err != nil {
		return nil, err
	}

	return tag, nil
}

func (tagDAO *TagDataAccessObject) InsertTag(ctx context.Context, createTag models.CreateTagRequest, stage string) (*models.Tag, error) {
	tag := models.Tag{
		Id:          uuid.NewString(),
		Name:        createTag.Name,
		Description: createTag.Description,
	}

	item, err := attributevalue.MarshalMap(tag)
	if err != nil {
		return nil, err
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(constants.GetDBName(stage)),
		Item:      item,
	}
	_, err = tagDAO.db.PutItem(ctx, input)
	if err != nil {
		return nil, err
	}

	return &tag, nil
}

func (tagDAO *TagDataAccessObject) UpdateTag(ctx context.Context, id string, updateTag models.UpdateTagRequest, stage string) (*models.Tag, error) {
	key, err := attributevalue.Marshal(id)
	if err != nil {
		return nil, err
	}

	expr, err := expression.NewBuilder().WithUpdate(
		expression.Set(
			expression.Name("name"),
			expression.Value(updateTag.Name),
		).Set(
			expression.Name("description"),
			expression.Value(updateTag.Description),
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
		ReturnValues:              dynamodbTypes.ReturnValueAllNew,
	}

	res, err := tagDAO.db.UpdateItem(ctx, input)
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

	tag := new(models.Tag)
	err = attributevalue.UnmarshalMap(res.Attributes, tag)
	if err != nil {
		return nil, err
	}

	return tag, nil
}

func (tagDAO *TagDataAccessObject) DeleteTag(ctx context.Context, id string, stage string) (*models.Tag, error) {
	key, err := attributevalue.Marshal(id)
	if err != nil {
		return nil, err
	}

	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(constants.GetDBName(stage)),
		Key: map[string]dynamodbTypes.AttributeValue{
			"id": key,
		},
		ReturnValues: dynamodbTypes.ReturnValueAllOld,
	}

	res, err := tagDAO.db.DeleteItem(ctx, input)
	if err != nil {
		return nil, err
	}

	if res.Attributes == nil {
		return nil, nil
	}

	tag := new(models.Tag)
	err = attributevalue.UnmarshalMap(res.Attributes, tag)
	if err != nil {
		return nil, err
	}

	return tag, nil
}
