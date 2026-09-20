package persistence

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const (
	dailyUploadCount = 100
	dailyUploadBytes = 100 * 1024 * 1024
	maxUploadBytes   = 10 * 1024 * 1024
)

// ErrUploadQuota indicates that the UTC day's upload budget is exhausted.
var ErrUploadQuota = errors.New("daily image upload budget exhausted")

// ConsumeUpload reserves one upload and its normalized byte size in the single
// owner's daily budget. Call before writing media. Reservations are deliberately
// not refunded after a write failure because the write outcome can be uncertain.
func (s *Store) ConsumeUpload(ctx context.Context, size int64) error {
	return s.consumeUpload(ctx, size, time.Now())
}

func (s *Store) consumeUpload(ctx context.Context, size int64, now time.Time) error {
	if size <= 0 || size > maxUploadBytes {
		return fmt.Errorf("normalized image size must be between 1 and %d bytes", maxUploadBytes)
	}

	// Both counters are changed under the same condition. A read followed by an
	// unconditional update could allow concurrent requests to exceed the budget.
	_, err := s.DB.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.Table),
		Key: map[string]dynamodbTypes.AttributeValue{
			"id": &dynamodbTypes.AttributeValueMemberS{Value: "upload_budget:" + now.UTC().Format("2006-01-02")},
		},
		ConditionExpression: aws.String("(attribute_not_exists(#id) OR #type = :type) AND (attribute_not_exists(#uploads) OR #uploads < :max_uploads) AND (attribute_not_exists(#bytes) OR #bytes <= :remaining)"),
		UpdateExpression:    aws.String("SET #type = :type, #expires = :expires ADD #uploads :one, #bytes :size"),
		ExpressionAttributeNames: map[string]string{
			"#id": "id", "#type": "entity_type", "#uploads": "uploads", "#bytes": "bytes", "#expires": "expires_at",
		},
		ExpressionAttributeValues: map[string]dynamodbTypes.AttributeValue{
			":type":        &dynamodbTypes.AttributeValueMemberS{Value: "upload_budget"},
			":expires":     &dynamodbTypes.AttributeValueMemberN{Value: strconv.FormatInt(now.Add(72*time.Hour).Unix(), 10)},
			":one":         &dynamodbTypes.AttributeValueMemberN{Value: "1"},
			":size":        &dynamodbTypes.AttributeValueMemberN{Value: strconv.FormatInt(size, 10)},
			":max_uploads": &dynamodbTypes.AttributeValueMemberN{Value: strconv.Itoa(dailyUploadCount)},
			":remaining":   &dynamodbTypes.AttributeValueMemberN{Value: strconv.FormatInt(dailyUploadBytes-size, 10)},
		},
	})
	if err != nil {
		var conditionFailed *dynamodbTypes.ConditionalCheckFailedException
		if errors.As(err, &conditionFailed) {
			return ErrUploadQuota
		}
		return err
	}
	return nil
}
