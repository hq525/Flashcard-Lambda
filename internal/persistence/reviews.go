package persistence

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"time"

	"flashcard_lambda/internal/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go"
)

var ErrReviewConflict = errors.New("card review conflicts with stored state")

// ReviewStore keeps an immutable review and its resulting card schedule together.
type ReviewStore interface {
	GetCard(context.Context, string) (*models.Card, error)
	GetReview(context.Context, string, string) (*models.CardReview, error)
	SaveReview(context.Context, string, uint64, models.CardReview) error
	ListReviews(context.Context, string) ([]models.CardReview, error)
	DeleteReviews(context.Context, string) error
}

type dynamoReviewStore struct{ store *Store }

func NewReviewStore(store *Store) ReviewStore { return &dynamoReviewStore{store: store} }

// ReviewID scopes idempotency keys to a card without separator collisions.
func ReviewID(cardID, requestID string) string {
	return "card_review:" + base64.RawURLEncoding.EncodeToString([]byte(cardID)) + ":" + base64.RawURLEncoding.EncodeToString([]byte(requestID))
}

func (s *dynamoReviewStore) getConsistent(ctx context.Context, id string, out any) (bool, error) {
	key, err := idKey(id)
	if err != nil {
		return false, err
	}
	result, err := s.store.DB.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.store.Table), Key: key, ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return false, err
	}
	if len(result.Item) == 0 {
		return false, nil
	}
	if err := attributevalue.UnmarshalMap(result.Item, out); err != nil {
		return false, err
	}
	return true, nil
}

func (s *dynamoReviewStore) GetCard(ctx context.Context, id string) (*models.Card, error) {
	var card models.Card
	found, err := s.getConsistent(ctx, id, &card)
	if err != nil {
		return nil, err
	}
	if !found || card.EntityType != models.EntityTypeCard {
		return nil, nil
	}
	return &card, nil
}

func (s *dynamoReviewStore) GetReview(ctx context.Context, cardID, requestID string) (*models.CardReview, error) {
	var review models.CardReview
	found, err := s.getConsistent(ctx, ReviewID(cardID, requestID), &review)
	if err != nil {
		return nil, err
	}
	if !found || review.EntityType != models.EntityTypeCardReview {
		return nil, nil
	}
	if review.CardId != cardID || review.RequestId != requestID {
		return nil, errors.New("stored review does not match its card and request key")
	}
	return &review, nil
}

func (s *dynamoReviewStore) SaveReview(ctx context.Context, cardID string, revision uint64, review models.CardReview) error {
	key, err := idKey(cardID)
	if err != nil {
		return err
	}
	review.Id = ReviewID(cardID, review.RequestId)
	review.EntityType = models.EntityTypeCardReview
	review.CardId = cardID
	item, err := attributevalue.MarshalMap(review)
	if err != nil {
		return err
	}
	values, err := attributevalue.MarshalMap(map[string]any{
		":schedule": review.Schedule,
		":revision": review.Revision,
		":expected": revision,
		":reviewed": review.ReviewedAt,
		":correct":  review.Rating != models.RatingAgain,
		":card":     models.EntityTypeCard,
		":head":     review.Id,
	})
	if err != nil {
		return err
	}
	condition := "attribute_exists(#id) AND #type = :card AND attribute_not_exists(#deleting) AND #revision = :expected"
	if revision == 0 {
		condition = "attribute_exists(#id) AND #type = :card AND attribute_not_exists(#deleting) AND (attribute_not_exists(#revision) OR #revision = :expected)"
	}
	_, err = s.store.DB.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []dynamodbTypes.TransactWriteItem{
			{Update: &dynamodbTypes.Update{
				TableName: aws.String(s.store.Table), Key: key,
				UpdateExpression:    aws.String("SET #schedule = :schedule, #revision = :revision, #accessed = :reviewed, #correct = :correct, #updated = :reviewed, #head = :head"),
				ConditionExpression: aws.String(condition),
				ExpressionAttributeNames: map[string]string{
					"#id": "id", "#type": "entity_type", "#schedule": "schedule", "#revision": "review_revision",
					"#accessed": "last_accessed_date_time", "#correct": "previously_correct", "#updated": "updated_date_time",
					"#head": "last_review_id", "#deleting": "review_deleting",
				},
				ExpressionAttributeValues: values,
			}},
			{Put: &dynamodbTypes.Put{
				TableName: aws.String(s.store.Table), Item: item,
				ConditionExpression:      aws.String("attribute_not_exists(#id)"),
				ExpressionAttributeNames: map[string]string{"#id": "id"},
			}},
		},
	})
	if err == nil {
		return nil
	}
	var cancelled *dynamodbTypes.TransactionCanceledException
	if errors.As(err, &cancelled) {
		for _, reason := range cancelled.CancellationReasons {
			if aws.ToString(reason.Code) == "ConditionalCheckFailed" || aws.ToString(reason.Code) == "TransactionConflict" {
				return fmt.Errorf("%w: %w", ErrReviewConflict, err)
			}
		}
	}
	var conflict smithy.APIError
	if errors.As(err, &conflict) && conflict.ErrorCode() == "TransactionConflictException" {
		return fmt.Errorf("%w: %w", ErrReviewConflict, err)
	}
	return err
}
func (s *dynamoReviewStore) ListReviews(ctx context.Context, cardID string) ([]models.CardReview, error) {
	input := &dynamodb.QueryInput{
		TableName: aws.String(s.store.Table), IndexName: aws.String(IndexCardID),
		KeyConditionExpression:   aws.String("#card = :card"),
		FilterExpression:         aws.String("#type = :type"),
		ExpressionAttributeNames: map[string]string{"#card": "card_id", "#type": "entity_type"},
		ExpressionAttributeValues: map[string]dynamodbTypes.AttributeValue{
			":card": &dynamodbTypes.AttributeValueMemberS{Value: cardID},
			":type": &dynamodbTypes.AttributeValueMemberS{Value: models.EntityTypeCardReview},
		},
	}
	reviews := make([]models.CardReview, 0)
	for {
		result, err := s.store.DB.Query(ctx, input)
		if err != nil {
			return nil, err
		}
		var page []models.CardReview
		if err := attributevalue.UnmarshalListOfMaps(result.Items, &page); err != nil {
			return nil, err
		}
		reviews = append(reviews, page...)
		if len(result.LastEvaluatedKey) == 0 {
			break
		}
		input.ExclusiveStartKey = result.LastEvaluatedKey
	}
	// RFC3339Nano strings do not sort chronologically when fractional seconds
	// have different lengths, so compare parsed instants.
	instants := make(map[string]time.Time, len(reviews))
	for _, review := range reviews {
		reviewedAt, err := time.Parse(time.RFC3339Nano, review.ReviewedAt)
		if err != nil {
			return nil, fmt.Errorf("review %q has invalid reviewed_at: %w", review.Id, err)
		}
		instants[review.ReviewedAt] = reviewedAt
	}
	sort.Slice(reviews, func(i, j int) bool {
		a, b := instants[reviews[i].ReviewedAt], instants[reviews[j].ReviewedAt]
		if !a.Equal(b) {
			return a.Before(b)
		}
		if reviews[i].Revision != reviews[j].Revision {
			return reviews[i].Revision < reviews[j].Revision
		}
		return reviews[i].Id < reviews[j].Id
	})
	return reviews, nil
}

func (s *dynamoReviewStore) DeleteReviews(ctx context.Context, cardID string) error {
	key, err := idKey(cardID)
	if err != nil {
		return err
	}
	// Lock out new reviews before reading the head. Returning ALL_NEW makes
	// the pointer current even when a review committed just before this update.
	marked, err := s.store.DB.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.store.Table), Key: key,
		UpdateExpression:         aws.String("SET #deleting = :deleting"),
		ConditionExpression:      aws.String("attribute_exists(#id) AND #type = :card"),
		ExpressionAttributeNames: map[string]string{"#id": "id", "#type": "entity_type", "#deleting": "review_deleting"},
		ExpressionAttributeValues: map[string]dynamodbTypes.AttributeValue{
			":deleting": &dynamodbTypes.AttributeValueMemberBOOL{Value: true},
			":card":     &dynamodbTypes.AttributeValueMemberS{Value: models.EntityTypeCard},
		},
		ReturnValues: dynamodbTypes.ReturnValueAllNew,
	})
	if err != nil {
		var missing *dynamodbTypes.ConditionalCheckFailedException
		if errors.As(err, &missing) {
			return nil
		}
		return err
	}
	var card models.Card
	if err := attributevalue.UnmarshalMap(marked.Attributes, &card); err != nil {
		return err
	}
	if card.Id != cardID || card.EntityType != models.EntityTypeCard {
		return errors.New("review cleanup did not return the marked card")
	}
	// The immutable predecessor chain avoids eventually consistent GSI reads.
	// A failed cleanup leaves the marker set, and each successful transaction
	// saves its progress so another delete request can resume from the head.
	for head := card.LastReviewId; head != ""; {
		var review models.CardReview
		found, err := s.getConsistent(ctx, head, &review)
		if err != nil {
			return err
		}
		if !found || review.Id != head || review.CardId != cardID || review.EntityType != models.EntityTypeCardReview {
			return fmt.Errorf("review cleanup found missing or invalid history at %q", head)
		}
		reviewKey, err := idKey(head)
		if err != nil {
			return err
		}
		update := "REMOVE #head"
		values := map[string]dynamodbTypes.AttributeValue{
			":head":     &dynamodbTypes.AttributeValueMemberS{Value: head},
			":card":     &dynamodbTypes.AttributeValueMemberS{Value: models.EntityTypeCard},
			":deleting": &dynamodbTypes.AttributeValueMemberBOOL{Value: true},
		}
		if review.PreviousReviewId != "" {
			update = "SET #head = :previous"
			values[":previous"] = &dynamodbTypes.AttributeValueMemberS{Value: review.PreviousReviewId}
		}
		_, err = s.store.DB.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
			TransactItems: []dynamodbTypes.TransactWriteItem{
				{Update: &dynamodbTypes.Update{
					TableName: aws.String(s.store.Table), Key: key,
					UpdateExpression:          aws.String(update),
					ConditionExpression:       aws.String("#type = :card AND #deleting = :deleting AND #head = :head"),
					ExpressionAttributeNames:  map[string]string{"#type": "entity_type", "#deleting": "review_deleting", "#head": "last_review_id"},
					ExpressionAttributeValues: values,
				}},
				{Delete: &dynamodbTypes.Delete{
					TableName: aws.String(s.store.Table), Key: reviewKey,
					ConditionExpression:      aws.String("#type = :type AND #card = :card"),
					ExpressionAttributeNames: map[string]string{"#type": "entity_type", "#card": "card_id"},
					ExpressionAttributeValues: map[string]dynamodbTypes.AttributeValue{
						":type": &dynamodbTypes.AttributeValueMemberS{Value: models.EntityTypeCardReview},
						":card": &dynamodbTypes.AttributeValueMemberS{Value: cardID},
					},
				}},
			},
		})
		if err != nil {
			return err
		}
		head = review.PreviousReviewId
	}
	return nil
}
