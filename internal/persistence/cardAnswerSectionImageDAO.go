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

type ICardAnswerSectionImageDataAccessObject interface {
	GetCardAnswerSectionImages(ctx context.Context, cardAnswerSectionId string) ([]models.CardAnswerSectionImage, error)
	GetCardAnswerSectionImage(ctx context.Context, id string) (*models.CardAnswerSectionImage, error)
	InsertCardAnswerSectionImage(ctx context.Context, req models.CreateCardAnswerSectionImageRequest) (*models.CardAnswerSectionImage, error)
	UpdateCardAnswerSectionImage(ctx context.Context, id string, req models.UpdateCardAnswerSectionImageRequest) (*models.CardAnswerSectionImage, error)
	DeleteCardAnswerSectionImage(ctx context.Context, id string) (*models.CardAnswerSectionImage, error)
}

type CardAnswerSectionImageDataAccessObject struct {
	db *dynamodb.Client
}

func NewCardAnswerSectionImageDataAccessObject(db *dynamodb.Client) ICardAnswerSectionImageDataAccessObject {
	return &CardAnswerSectionImageDataAccessObject{db: db}
}

func (dao *CardAnswerSectionImageDataAccessObject) GetCardAnswerSectionImages(ctx context.Context, cardAnswerSectionId string) ([]models.CardAnswerSectionImage, error) {
	expr, err := expression.NewBuilder().WithFilter(
		expression.Equal(
			expression.Name("card_answer_section_id"),
			expression.Value(cardAnswerSectionId),
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

	var images []models.CardAnswerSectionImage
	for {
		res, err := dao.db.Scan(ctx, input)
		if err != nil {
			return nil, err
		}

		var page []models.CardAnswerSectionImage
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

func (dao *CardAnswerSectionImageDataAccessObject) GetCardAnswerSectionImage(ctx context.Context, id string) (*models.CardAnswerSectionImage, error) {
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

	image := new(models.CardAnswerSectionImage)
	err = attributevalue.UnmarshalMap(result.Item, image)
	if err != nil {
		return nil, err
	}

	return image, nil
}

func (dao *CardAnswerSectionImageDataAccessObject) InsertCardAnswerSectionImage(ctx context.Context, req models.CreateCardAnswerSectionImageRequest) (*models.CardAnswerSectionImage, error) {
	image := models.CardAnswerSectionImage{
		Id:                  uuid.NewString(),
		CardAnswerSectionId: req.CardAnswerSectionId,
		SequenceNumber:      req.SequenceNumber,
		ImageURL:            req.ImageURL,
		CreatedDateTime:     time.Now().UTC().Format(time.RFC3339),
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

func (dao *CardAnswerSectionImageDataAccessObject) UpdateCardAnswerSectionImage(ctx context.Context, id string, req models.UpdateCardAnswerSectionImageRequest) (*models.CardAnswerSectionImage, error) {
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

	image := new(models.CardAnswerSectionImage)
	err = attributevalue.UnmarshalMap(res.Attributes, image)
	if err != nil {
		return nil, err
	}

	return image, nil
}

func (dao *CardAnswerSectionImageDataAccessObject) DeleteCardAnswerSectionImage(ctx context.Context, id string) (*models.CardAnswerSectionImage, error) {
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

	image := new(models.CardAnswerSectionImage)
	err = attributevalue.UnmarshalMap(res.Attributes, image)
	if err != nil {
		return nil, err
	}

	return image, nil
}
