package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"

	"flashcard_lambda/internal/constants"
	"flashcard_lambda/internal/models"
)

type ICardQuestionImageDataAccessObject interface {
	GetCardQuestionImages(ctx context.Context, cardId string) ([]models.CardQuestionImage, error)
	GetCardQuestionImage(ctx context.Context, id string) (*models.CardQuestionImage, error)
	InsertCardQuestionImage(ctx context.Context, req models.CreateCardQuestionImageRequest) (*models.CardQuestionImage, error)
	UpdateCardQuestionImage(ctx context.Context, id string, req models.UpdateCardQuestionImageRequest) (*models.CardQuestionImage, error)
	DeleteCardQuestionImage(ctx context.Context, id string) (*models.CardQuestionImage, error)
}

type CardQuestionImageDataAccessObject struct {
	db *dynamodb.Client
}

func NewCardQuestionImageDataAccessObject(db *dynamodb.Client) ICardQuestionImageDataAccessObject {
	return &CardQuestionImageDataAccessObject{db: db}
}

func (dao *CardQuestionImageDataAccessObject) GetCardQuestionImages(ctx context.Context, cardId string) ([]models.CardQuestionImage, error) {
	expr, err := expression.NewBuilder().WithFilter(
		expression.Equal(
			expression.Name("card_id"),
			expression.Value(cardId),
		),
	).Build()
	if err != nil {
		return nil, err
	}

	input := &dynamodb.ScanInput{
		TableName:                 aws.String(constants.GetDBName()),
		FilterExpression:          expr.Filter(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	}

	var images []models.CardQuestionImage
	for {
		res, err := dao.db.Scan(ctx, input)
		if err != nil {
			return nil, err
		}

		var page []models.CardQuestionImage
		if err = attributevalue.UnmarshalListOfMaps(res.Items, &page); err != nil {
			return nil, err
		}
		images = append(images, page...)

		if res.LastEvaluatedKey == nil {
			break
		}
		input.ExclusiveStartKey = res.LastEvaluatedKey
	}

	return images, nil
}

func (dao *CardQuestionImageDataAccessObject) GetCardQuestionImage(ctx context.Context, id string) (*models.CardQuestionImage, error) {
	key, err := attributevalue.Marshal(id)
	if err != nil {
		return nil, err
	}

	input := &dynamodb.GetItemInput{
		TableName: aws.String(constants.GetDBName()),
		Key: map[string]dynamodbTypes.AttributeValue{
			"id": key,
		},
	}

	result, err := dao.db.GetItem(ctx, input)
	if err != nil {
		return nil, err
	}

	if result.Item == nil {
		return nil, nil
	}

	image := new(models.CardQuestionImage)
	err = attributevalue.UnmarshalMap(result.Item, image)
	if err != nil {
		return nil, err
	}

	return image, nil
}

func (dao *CardQuestionImageDataAccessObject) InsertCardQuestionImage(ctx context.Context, req models.CreateCardQuestionImageRequest) (*models.CardQuestionImage, error) {
	image := models.CardQuestionImage{
		Id:              uuid.NewString(),
		CardId:          req.CardId,
		SequenceNumber:  req.SequenceNumber,
		ImageURL:        req.ImageURL,
		CreatedDateTime: time.Now().UTC().Format(time.RFC3339),
	}

	item, err := attributevalue.MarshalMap(image)
	if err != nil {
		return nil, err
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(constants.GetDBName()),
		Item:      item,
	}
	_, err = dao.db.PutItem(ctx, input)
	if err != nil {
		return nil, err
	}

	return &image, nil
}

func (dao *CardQuestionImageDataAccessObject) UpdateCardQuestionImage(ctx context.Context, id string, req models.UpdateCardQuestionImageRequest) (*models.CardQuestionImage, error) {
	key, err := attributevalue.Marshal(id)
	if err != nil {
		return nil, err
	}

	expr, err := expression.NewBuilder().WithUpdate(
		expression.Set(
			expression.Name("sequence_number"),
			expression.Value(req.SequenceNumber),
		).Set(
			expression.Name("image_url"),
			expression.Value(req.ImageURL),
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
		TableName:                 aws.String(constants.GetDBName()),
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		ConditionExpression:       expr.Condition(),
		ReturnValues:              dynamodbTypes.ReturnValueAllNew,
	}

	res, err := dao.db.UpdateItem(ctx, input)
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

	image := new(models.CardQuestionImage)
	err = attributevalue.UnmarshalMap(res.Attributes, image)
	if err != nil {
		return nil, err
	}

	return image, nil
}

func (dao *CardQuestionImageDataAccessObject) DeleteCardQuestionImage(ctx context.Context, id string) (*models.CardQuestionImage, error) {
	key, err := attributevalue.Marshal(id)
	if err != nil {
		return nil, err
	}

	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(constants.GetDBName()),
		Key: map[string]dynamodbTypes.AttributeValue{
			"id": key,
		},
		ReturnValues: dynamodbTypes.ReturnValueAllOld,
	}

	res, err := dao.db.DeleteItem(ctx, input)
	if err != nil {
		return nil, err
	}

	if res.Attributes == nil {
		return nil, nil
	}

	image := new(models.CardQuestionImage)
	err = attributevalue.UnmarshalMap(res.Attributes, image)
	if err != nil {
		return nil, err
	}

	return image, nil
}
