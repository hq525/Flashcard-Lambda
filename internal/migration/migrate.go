package migration

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"

	"flashcard_lambda/internal/models"
	"flashcard_lambda/internal/storage"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/google/uuid"
)

const (
	DefaultMaxPages  = 10000
	maxReportEntries = 100000
)

var tablePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{3,255}$`)

type Config struct {
	Table         string
	Bucket        string
	SourceBuckets []string
	Apply         bool
	MaxPages      int
}

// Validate checks configuration before the command contacts AWS. Only a dry run
// may omit SourceBuckets; then the destination bucket is the sole allowed source.
func (c Config) Validate() error {
	if !tablePattern.MatchString(c.Table) {
		return errors.New("a valid --table is required")
	}
	if !validBucket(c.Bucket) {
		return errors.New("a valid --bucket is required")
	}
	if c.Apply && len(c.SourceBuckets) == 0 {
		return errors.New("--apply requires an explicit --source-buckets allowlist")
	}
	for _, bucket := range c.SourceBuckets {
		if !validBucket(bucket) {
			return errors.New("--source-buckets must contain valid S3 bucket names")
		}
	}
	if c.MaxPages < 0 || c.MaxPages > DefaultMaxPages {
		return errors.New("--max-pages must be between 1 and 10000")
	}
	return nil
}

type DynamoClient interface {
	Scan(context.Context, *dynamodb.ScanInput, ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	UpdateItem(context.Context, *dynamodb.UpdateItemInput, ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
}
type ObjectReader interface {
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}
type ObjectWriter interface {
	Put(context.Context, string, []byte, string) error
}

// Reports contain canonical IDs and fixed action/error codes only. They never
// include source URLs, source keys, object bytes, or underlying SDK errors.
type Entry struct {
	ID         string `json:"id,omitempty"`
	EntityType string `json:"entityType,omitempty"`
	Action     string `json:"action"`
	Error      string `json:"error,omitempty"`
}
type Report struct {
	Mode     string  `json:"mode"`
	Pages    int     `json:"pages"`
	Examined int     `json:"examined"`
	Planned  int     `json:"planned"`
	Migrated int     `json:"migrated"`
	Skipped  int     `json:"skipped"`
	Errors   int     `json:"errors"`
	Entries  []Entry `json:"entries"`
}

type imageRecordMetadata struct {
	ID         string `dynamodbav:"id"`
	EntityType string `dynamodbav:"entity_type"`
	LegacyURL  string `dynamodbav:"image_url"`
	StorageKey string `dynamodbav:"storage_key"`
	CardID     string `dynamodbav:"card_id"`
	SectionID  string `dynamodbav:"card_answer_section_id"`
}

// Run scans only image metadata and processes one image at a time. A page cap
// bounds accidental broad scans; exceeding it is explicit, never a clean result.
func Run(ctx context.Context, cfg Config, db DynamoClient, objects ObjectReader, writer ObjectWriter) (Report, error) {
	report := Report{Mode: "dry-run", Entries: []Entry{}}
	if cfg.Apply {
		report.Mode = "apply"
	}
	if err := cfg.Validate(); err != nil {
		return report, err
	}
	if db == nil || cfg.Apply && (objects == nil || writer == nil) {
		return report, errors.New("migration clients are not configured")
	}
	if cfg.MaxPages == 0 {
		cfg.MaxPages = DefaultMaxPages
	}
	if len(cfg.SourceBuckets) == 0 {
		cfg.SourceBuckets = []string{cfg.Bucket}
	}
	scan := &dynamodb.ScanInput{
		TableName: aws.String(cfg.Table), Limit: aws.Int32(100), ConsistentRead: aws.Bool(true),
		ProjectionExpression:      aws.String("#id, #type, #legacy, #key, #card, #section"),
		FilterExpression:          aws.String("#type = :question OR #type = :section"),
		ExpressionAttributeNames:  map[string]string{"#id": "id", "#type": "entity_type", "#legacy": "image_url", "#key": "storage_key", "#card": "card_id", "#section": "card_answer_section_id"},
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{":question": &ddbtypes.AttributeValueMemberS{Value: models.EntityTypeCardQuestionImage}, ":section": &ddbtypes.AttributeValueMemberS{Value: models.EntityTypeCardAnswerSectionImage}},
	}
	for report.Pages < cfg.MaxPages {
		result, err := db.Scan(ctx, scan)
		if err != nil || result == nil {
			return report, errors.New("image metadata scan failed")
		}
		report.Pages++
		for _, item := range result.Items {
			if len(report.Entries) >= maxReportEntries {
				return report, errors.New("image report entry limit reached; migration is incomplete")
			}
			entry := migrateRecord(ctx, cfg, db, objects, writer, item)
			report.Examined++
			switch entry.Action {
			case "planned":
				report.Planned++
			case "migrated":
				report.Migrated++
			case "skipped":
				report.Skipped++
			case "error":
				report.Errors++
			}
			report.Entries = append(report.Entries, entry)
		}
		if len(result.LastEvaluatedKey) == 0 {
			return report, nil
		}
		scan.ExclusiveStartKey = result.LastEvaluatedKey
	}
	return report, errors.New("image scan page limit reached; migration is incomplete")
}

func migrateRecord(ctx context.Context, cfg Config, db DynamoClient, objects ObjectReader, writer ObjectWriter, item map[string]ddbtypes.AttributeValue) Entry {
	entry := Entry{Action: "error"}
	var record imageRecordMetadata
	if err := attributevalue.UnmarshalMap(item, &record); err != nil {
		entry.Error = "invalid_record"
		return entry
	}
	parsed, err := uuid.Parse(record.ID)
	if err != nil || parsed.String() != record.ID {
		entry.Error = "invalid_id"
		return entry
	}
	entry.ID = record.ID
	switch record.EntityType {
	case models.EntityTypeCardQuestionImage, models.EntityTypeCardAnswerSectionImage:
		entry.EntityType = record.EntityType
	default:
		entry.Error = "invalid_entity"
		return entry
	}
	if _, hasKey := item["storage_key"]; hasKey {
		if err := storage.ValidateImageKey(record.ID, record.StorageKey); err != nil {
			entry.Error = "invalid_managed_key"
			return entry
		}
		entry.Action = "skipped"
		return entry
	}
	parent := record.CardID
	if record.EntityType == models.EntityTypeCardAnswerSectionImage {
		parent = record.SectionID
	}
	if strings.TrimSpace(parent) == "" || len(parent) > 128 {
		entry.Error = "invalid_parent"
		return entry
	}
	source, err := ParseLegacyURL(record.LegacyURL, cfg.SourceBuckets)
	if err != nil {
		entry.Error = "invalid_source"
		return entry
	}
	if !cfg.Apply {
		entry.Action = "planned"
		return entry
	}
	data, err := readBoundedObject(ctx, objects, source.Bucket, source.Key, storage.MaxLegacyImageBytes)
	if err != nil {
		entry.Error = "source_read_failed"
		if errors.Is(err, storage.ErrImageTooLarge) {
			entry.Error = "source_too_large"
		}
		return entry
	}
	normalized, err := storage.NormalizeImage(data, http.DetectContentType(data))
	if err != nil {
		entry.Error = "invalid_image"
		return entry
	}
	key, err := storage.ImageKey(record.ID, normalized.ContentType)
	if err != nil {
		entry.Error = "invalid_managed_key"
		return entry
	}
	if err := writer.Put(ctx, key, normalized.Data, normalized.ContentType); err != nil {
		if !isExistingObject(err) {
			entry.Error = "target_write_failed"
			return entry
		}
		existing, err := readBoundedObject(ctx, objects, cfg.Bucket, key, storage.MaxStoredImageBytes)
		if err != nil {
			entry.Error = "target_read_failed"
			return entry
		}
		if !bytes.Equal(existing, normalized.Data) {
			entry.Error = "target_conflict"
			return entry
		}
	}
	_, err = db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(cfg.Table), Key: map[string]ddbtypes.AttributeValue{"id": &ddbtypes.AttributeValueMemberS{Value: record.ID}},
		UpdateExpression:          aws.String("SET #key = :key"),
		ConditionExpression:       aws.String("attribute_exists(#id) AND #type = :type AND #legacy = :legacy AND attribute_not_exists(#key)"),
		ExpressionAttributeNames:  map[string]string{"#id": "id", "#type": "entity_type", "#legacy": "image_url", "#key": "storage_key"},
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{":key": &ddbtypes.AttributeValueMemberS{Value: key}, ":type": &ddbtypes.AttributeValueMemberS{Value: record.EntityType}, ":legacy": &ddbtypes.AttributeValueMemberS{Value: record.LegacyURL}},
	})
	if err != nil {
		entry.Error = "record_update_failed"
		var changed *ddbtypes.ConditionalCheckFailedException
		if errors.As(err, &changed) {
			entry.Error = "record_changed"
		}
		return entry
	}
	entry.Action = "migrated"
	return entry
}

func isExistingObject(err error) bool {
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && (apiErr.ErrorCode() == "PreconditionFailed" || apiErr.ErrorCode() == "ConditionalRequestConflict")
}

func readBoundedObject(ctx context.Context, objects ObjectReader, bucket, key string, maxBytes int64) ([]byte, error) {
	result, err := objects.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return nil, err
	}
	if result == nil || result.Body == nil {
		return nil, errors.New("object response has no body")
	}
	defer result.Body.Close()
	if aws.ToInt64(result.ContentLength) > maxBytes {
		return nil, storage.ErrImageTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(result.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, storage.ErrImageTooLarge
	}
	return data, nil
}
