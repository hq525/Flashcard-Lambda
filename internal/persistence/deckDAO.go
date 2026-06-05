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

type IDeckDataAccessObject interface {
	GetDecks(ctx context.Context, categoryId string) ([]models.Deck, error)
	GetDeck(ctx context.Context, id string) (*models.Deck, error)
	InsertDeck(ctx context.Context, createDeck models.CreateDeckRequest) (*models.Deck, error)
	UpdateDeck(ctx context.Context, id string, updateDeck models.UpdateDeckRequest) (*models.Deck, error)
	DeleteDeck(ctx context.Context, id string) (*models.Deck, error)
}

type DeckDataAccessObject struct {
	db *dynamodb.Client
}

func NewDeckDataAccessObject(db *dynamodb.Client) IDeckDataAccessObject {
	return &DeckDataAccessObject{
		db: db,
	}
}

func (deckDAO *DeckDataAccessObject) GetDecks(ctx context.Context, categoryId string) ([]models.Deck, error) {
	expr, err := expression.NewBuilder().WithFilter(
		expression.Equal(
			expression.Name("category_id"),
			expression.Value(categoryId),
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

	var decks []models.Deck
	for {
		res, err := deckDAO.db.Scan(ctx, input)
		if err != nil {
			return nil, err
		}

		var page []models.Deck
		if err = attributevalue.UnmarshalListOfMaps(res.Items, &page); err != nil {
			return nil, err
		}
		decks = append(decks, page...)

		if res.LastEvaluatedKey == nil {
			break
		}
		input.ExclusiveStartKey = res.LastEvaluatedKey
	}

	return decks, nil
}

func (deckDAO *DeckDataAccessObject) GetDeck(ctx context.Context, id string) (*models.Deck, error) {
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

	result, err := deckDAO.db.GetItem(ctx, input)
	if err != nil {
		return nil, err
	}

	if result.Item == nil {
		return nil, nil
	}

	deck := new(models.Deck)
	err = attributevalue.UnmarshalMap(result.Item, deck)
	if err != nil {
		return nil, err
	}

	return deck, nil
}

func (deckDAO *DeckDataAccessObject) InsertDeck(ctx context.Context, createDeck models.CreateDeckRequest) (*models.Deck, error) {
	deck := models.Deck{
		Id:          uuid.NewString(),
		CategoryId:  createDeck.CategoryId,
		Name:        createDeck.Name,
		Description: createDeck.Description,
	}

	item, err := attributevalue.MarshalMap(deck)
	if err != nil {
		return nil, err
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(constants.GetDBName()),
		Item:      item,
	}
	_, err = deckDAO.db.PutItem(ctx, input)
	if err != nil {
		return nil, err
	}

	return &deck, nil
}

func (deckDAO *DeckDataAccessObject) UpdateDeck(ctx context.Context, id string, updateDeck models.UpdateDeckRequest) (*models.Deck, error) {
	key, err := attributevalue.Marshal(id)
	if err != nil {
		return nil, err
	}

	expr, err := expression.NewBuilder().WithUpdate(
		expression.Set(
			expression.Name("name"),
			expression.Value(updateDeck.Name),
		).Set(
			expression.Name("description"),
			expression.Value(updateDeck.Description),
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

	res, err := deckDAO.db.UpdateItem(ctx, input)
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

	deck := new(models.Deck)
	err = attributevalue.UnmarshalMap(res.Attributes, deck)
	if err != nil {
		return nil, err
	}

	return deck, nil
}

func (deckDAO *DeckDataAccessObject) DeleteDeck(ctx context.Context, id string) (*models.Deck, error) {
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

	res, err := deckDAO.db.DeleteItem(ctx, input)
	if err != nil {
		return nil, err
	}

	if res.Attributes == nil {
		return nil, nil
	}

	deck := new(models.Deck)
	err = attributevalue.UnmarshalMap(res.Attributes, deck)
	if err != nil {
		return nil, err
	}

	return deck, nil
}
