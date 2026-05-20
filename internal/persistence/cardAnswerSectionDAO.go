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

type ICardAnswerSectionDataAccessObject interface {
	GetCardAnswerSections(ctx context.Context, cardId string, stage string) ([]models.CardAnswerSection, error)
	GetCardAnswerSection(ctx context.Context, id string, stage string) (*models.CardAnswerSection, error)
	InsertCardAnswerSection(ctx context.Context, req models.CreateCardAnswerSectionRequest, stage string) (*models.CardAnswerSection, error)
	UpdateCardAnswerSection(ctx context.Context, id string, req models.UpdateCardAnswerSectionRequest, stage string) (*models.CardAnswerSection, error)
	DeleteCardAnswerSection(ctx context.Context, id string, stage string) (*models.CardAnswerSection, error)
}

type CardAnswerSectionDataAccessObject struct {
	db *dynamodb.Client
}

func NewCardAnswerSectionDataAccessObject(db *dynamodb.Client) ICardAnswerSectionDataAccessObject {
	return &CardAnswerSectionDataAccessObject{db: db}
}

func (dao *CardAnswerSectionDataAccessObject) GetCardAnswerSections(ctx context.Context, cardId string, stage string) ([]models.CardAnswerSection, error) {
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
		TableName:                 aws.String(constants.GetDBName(stage)),
		FilterExpression:          expr.Filter(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	}

	var sections []models.CardAnswerSection
	for {
		res, err := dao.db.Scan(ctx, input)
		if err != nil {
			return nil, err
		}

		var page []models.CardAnswerSection
		if err = attributevalue.UnmarshalListOfMaps(res.Items, &page); err != nil {
			return nil, err
		}
		sections = append(sections, page...)

		if res.LastEvaluatedKey == nil {
			break
		}
		input.ExclusiveStartKey = res.LastEvaluatedKey
	}

	return sections, nil
}

func (dao *CardAnswerSectionDataAccessObject) GetCardAnswerSection(ctx context.Context, id string, stage string) (*models.CardAnswerSection, error) {
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

	result, err := dao.db.GetItem(ctx, input)
	if err != nil {
		return nil, err
	}

	if result.Item == nil {
		return nil, nil
	}

	section := new(models.CardAnswerSection)
	err = attributevalue.UnmarshalMap(result.Item, section)
	if err != nil {
		return nil, err
	}

	return section, nil
}

func (dao *CardAnswerSectionDataAccessObject) InsertCardAnswerSection(ctx context.Context, req models.CreateCardAnswerSectionRequest, stage string) (*models.CardAnswerSection, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	section := models.CardAnswerSection{
		Id:              uuid.NewString(),
		CardId:          req.CardId,
		SequenceNumber:  req.SequenceNumber,
		Title:           req.Title,
		Answer:          req.Answer,
		CreatedDateTime: now,
		UpdatedDateTime: now,
	}

	item, err := attributevalue.MarshalMap(section)
	if err != nil {
		return nil, err
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(constants.GetDBName(stage)),
		Item:      item,
	}
	_, err = dao.db.PutItem(ctx, input)
	if err != nil {
		return nil, err
	}

	return &section, nil
}

func (dao *CardAnswerSectionDataAccessObject) UpdateCardAnswerSection(ctx context.Context, id string, req models.UpdateCardAnswerSectionRequest, stage string) (*models.CardAnswerSection, error) {
	key, err := attributevalue.Marshal(id)
	if err != nil {
		return nil, err
	}

	expr, err := expression.NewBuilder().WithUpdate(
		expression.Set(
			expression.Name("sequence_number"),
			expression.Value(req.SequenceNumber),
		).Set(
			expression.Name("title"),
			expression.Value(req.Title),
		).Set(
			expression.Name("answer"),
			expression.Value(req.Answer),
		).Set(
			expression.Name("updated_date_time"),
			expression.Value(time.Now().UTC().Format(time.RFC3339)),
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

	section := new(models.CardAnswerSection)
	err = attributevalue.UnmarshalMap(res.Attributes, section)
	if err != nil {
		return nil, err
	}

	return section, nil
}

func (dao *CardAnswerSectionDataAccessObject) DeleteCardAnswerSection(ctx context.Context, id string, stage string) (*models.CardAnswerSection, error) {
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

	res, err := dao.db.DeleteItem(ctx, input)
	if err != nil {
		return nil, err
	}

	if res.Attributes == nil {
		return nil, nil
	}

	section := new(models.CardAnswerSection)
	err = attributevalue.UnmarshalMap(res.Attributes, section)
	if err != nil {
		return nil, err
	}

	return section, nil
}
