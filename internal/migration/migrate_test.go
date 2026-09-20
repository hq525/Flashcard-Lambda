package migration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"strings"
	"testing"

	"flashcard_lambda/internal/models"
	"flashcard_lambda/internal/storage"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

const imageID = "8c47d801-dd96-449c-8b47-7fe3f7fb917e"
const legacyURL = "https://legacy-bucket.s3.amazonaws.com/question-images/original.png"
const targetKey = "images/8c47d801-dd96-449c-8b47-7fe3f7fb917e.png"

func imageRecord() map[string]ddbtypes.AttributeValue {
	return map[string]ddbtypes.AttributeValue{
		"id":          &ddbtypes.AttributeValueMemberS{Value: imageID},
		"entity_type": &ddbtypes.AttributeValueMemberS{Value: models.EntityTypeCardQuestionImage},
		"image_url":   &ddbtypes.AttributeValueMemberS{Value: legacyURL},
		"card_id":     &ddbtypes.AttributeValueMemberS{Value: "card-parent"},
	}
}

type migrationDB struct {
	t                     *testing.T
	item                  map[string]ddbtypes.AttributeValue
	scans, updates        int
	changeURL, failUpdate bool
}

func (d *migrationDB) Scan(_ context.Context, input *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	d.scans++
	if aws.ToInt32(input.Limit) <= 0 || aws.ToInt32(input.Limit) > 100 {
		d.t.Error("scan did not bound its evaluated page size")
	}
	for _, field := range input.ExpressionAttributeNames {
		switch field {
		case "id", "entity_type", "image_url", "storage_key", "card_id", "card_answer_section_id":
		default:
			d.t.Errorf("unexpected projected metadata field %q", field)
		}
	}
	projection := aws.ToString(input.ProjectionExpression)
	for _, field := range []string{"question", "answer", "description"} {
		if strings.Contains(projection, field) {
			d.t.Error("scan projects private card text")
		}
	}
	if input.FilterExpression == nil || !strings.Contains(*input.FilterExpression, "OR") {
		d.t.Error("scan must filter the two image entity types")
	}
	result := make(map[string]ddbtypes.AttributeValue, len(d.item))
	for k, v := range d.item {
		result[k] = v
	}
	if d.changeURL {
		d.item["image_url"] = &ddbtypes.AttributeValueMemberS{Value: "https://legacy-bucket.s3.amazonaws.com/question-images/changed.png"}
	}
	return &dynamodb.ScanOutput{Items: []map[string]ddbtypes.AttributeValue{result}}, nil
}
func (d *migrationDB) UpdateItem(_ context.Context, input *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	d.updates++
	condition := aws.ToString(input.ConditionExpression)
	for alias, name := range input.ExpressionAttributeNames {
		condition = strings.ReplaceAll(condition, alias, name)
	}
	if !strings.Contains(condition, "attribute_exists(id)") || !strings.Contains(condition, "attribute_not_exists(storage_key)") || !strings.Contains(condition, "entity_type = :type") || !strings.Contains(condition, "image_url = :legacy") {
		d.t.Errorf("migration write does not guard source and record identity: %s", condition)
	}
	if input.UpdateExpression == nil || *input.UpdateExpression != "SET #key = :key" {
		d.t.Error("migration should update only storage_key")
	}
	if d.failUpdate {
		return nil, errors.New("backend includes private query token")
	}
	if d.item["image_url"].(*ddbtypes.AttributeValueMemberS).Value != input.ExpressionAttributeValues[":legacy"].(*ddbtypes.AttributeValueMemberS).Value {
		return nil, &ddbtypes.ConditionalCheckFailedException{}
	}
	d.item["storage_key"] = input.ExpressionAttributeValues[":key"]
	return &dynamodb.UpdateItemOutput{}, nil
}

type migrationObjects struct {
	t              *testing.T
	source, stored []byte
	reads, puts    int
	readErr        error
}

func (o *migrationObjects) GetObject(_ context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	o.reads++
	if o.readErr != nil {
		return nil, o.readErr
	}
	data := o.source
	switch aws.ToString(input.Bucket) {
	case "legacy-bucket":
		if aws.ToString(input.Key) != "question-images/original.png" {
			o.t.Errorf("wrong legacy key %q", aws.ToString(input.Key))
		}
	case "managed-bucket":
		if aws.ToString(input.Key) != targetKey {
			o.t.Error("existing target key is not bound to record ID")
		}
		data = o.stored
	default:
		o.t.Error("read escaped the configured source and destination buckets")
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(data)), ContentLength: aws.Int64(int64(len(data))), ContentType: aws.String("application/octet-stream")}, nil
}
func (o *migrationObjects) Put(_ context.Context, key string, data []byte, contentType string) error {
	o.puts++
	if key != targetKey || contentType != "image/png" {
		o.t.Errorf("unsafe target: %q %q", key, contentType)
	}
	if o.stored != nil {
		return &smithy.GenericAPIError{Code: "PreconditionFailed", Message: "target exists"}
	}
	o.stored = append([]byte(nil), data...)
	return nil
}
func migrationPNG(t *testing.T, red uint8) []byte {
	t.Helper()
	var buffer bytes.Buffer
	pixels := image.NewRGBA(image.Rect(0, 0, 2, 2))
	pixels.Set(0, 0, color.RGBA{R: red, A: 255})
	if err := png.Encode(&buffer, pixels); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
func migrationFixture(t *testing.T) (Config, *migrationDB, *migrationObjects) {
	return Config{Table: "flashcards", Bucket: "managed-bucket", SourceBuckets: []string{"legacy-bucket"}}, &migrationDB{t: t, item: imageRecord()}, &migrationObjects{t: t, source: migrationPNG(t, 100)}
}

func TestMigrationDryRunPlansWithoutReadingOrWritingMedia(t *testing.T) {
	cfg, db, objects := migrationFixture(t)
	report, err := Run(context.Background(), cfg, db, objects, objects)
	if err != nil || report.Planned != 1 || report.Errors != 0 || db.updates != 0 || objects.puts != 0 || objects.reads != 0 {
		t.Fatalf("dry run=%+v error=%v reads=%d writes=%d/%d", report, err, objects.reads, objects.puts, db.updates)
	}
}
func TestMigrationNormalizesAndAttachesWithoutRemovingOriginals(t *testing.T) {
	cfg, db, objects := migrationFixture(t)
	cfg.Apply = true
	objects.source = append(objects.source, []byte("private-trailing-content")...)
	original := append([]byte(nil), objects.source...)
	report, err := Run(context.Background(), cfg, db, objects, objects)
	if err != nil || report.Migrated != 1 || db.updates != 1 || objects.puts != 1 {
		t.Fatalf("migration=%+v err=%v", report, err)
	}
	if bytes.Contains(objects.stored, []byte("private-trailing-content")) || !bytes.Equal(objects.source, original) {
		t.Error("migration failed to normalize or preserved source incorrectly")
	}
	if db.item["image_url"].(*ddbtypes.AttributeValueMemberS).Value != legacyURL || db.item["storage_key"].(*ddbtypes.AttributeValueMemberS).Value != targetKey {
		t.Error("migration changed legacy location or failed to attach managed key")
	}
}
func TestMigrationRejectsInvalidRecordsWithoutSourceRequests(t *testing.T) {
	for name, edit := range map[string]func(map[string]ddbtypes.AttributeValue){
		"external URL": func(item map[string]ddbtypes.AttributeValue) {
			item["image_url"] = &ddbtypes.AttributeValueMemberS{Value: "https://attacker.example/payload?private=token"}
		},
		"malformed ID": func(item map[string]ddbtypes.AttributeValue) {
			item["id"] = &ddbtypes.AttributeValueMemberS{Value: "private-invalid-id"}
		},
		"foreign managed key": func(item map[string]ddbtypes.AttributeValue) {
			item["storage_key"] = &ddbtypes.AttributeValueMemberS{Value: "images/27d5a8fb-8a17-4554-9256-a8436bc48d66.png"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			cfg, db, objects := migrationFixture(t)
			cfg.Apply = true
			edit(db.item)
			report, err := Run(context.Background(), cfg, db, objects, objects)
			if err != nil || report.Errors != 1 || objects.reads != 0 || objects.puts != 0 || db.updates != 0 {
				t.Fatalf("invalid record=%+v err=%v reads=%d", report, err, objects.reads)
			}
			encoded, _ := json.Marshal(report)
			if bytes.Contains(encoded, []byte("private")) || bytes.Contains(encoded, []byte("attacker")) {
				t.Error("report disclosed unvalidated input")
			}
		})
	}
}
func TestMigrationSkipsValidMigratedRecords(t *testing.T) {
	cfg, db, objects := migrationFixture(t)
	cfg.Apply = true
	db.item["storage_key"] = &ddbtypes.AttributeValueMemberS{Value: targetKey}
	report, err := Run(context.Background(), cfg, db, objects, objects)
	if err != nil || report.Skipped != 1 || objects.reads != 0 || objects.puts != 0 || db.updates != 0 {
		t.Fatalf("rerun=%+v error=%v", report, err)
	}
}
func TestMigrationRejectsChangedLegacyURLAtConditionalAttach(t *testing.T) {
	cfg, db, objects := migrationFixture(t)
	cfg.Apply = true
	db.changeURL = true
	report, err := Run(context.Background(), cfg, db, objects, objects)
	if err != nil || report.Errors != 1 || report.Entries[0].Error != "record_changed" {
		t.Fatalf("changed source=%+v error=%v", report, err)
	}
	if _, exists := db.item["storage_key"]; exists {
		t.Fatal("changed source was overwritten")
	}
}
func TestMigrationResumesAfterCopyWhenNormalizedBytesMatch(t *testing.T) {
	cfg, db, objects := migrationFixture(t)
	cfg.Apply = true
	db.failUpdate = true
	first, err := Run(context.Background(), cfg, db, objects, objects)
	if err != nil || first.Errors != 1 || objects.stored == nil {
		t.Fatalf("first interrupted run=%+v %v", first, err)
	}
	db.failUpdate = false
	second, err := Run(context.Background(), cfg, db, objects, objects)
	if err != nil || second.Migrated != 1 || objects.reads != 3 {
		t.Fatalf("resumed run=%+v %v reads=%d", second, err, objects.reads)
	}
}
func TestMigrationNeverAttachesAConflictingExistingTarget(t *testing.T) {
	cfg, db, objects := migrationFixture(t)
	cfg.Apply = true
	objects.stored = migrationPNG(t, 200)
	before := append([]byte(nil), objects.stored...)
	report, err := Run(context.Background(), cfg, db, objects, objects)
	if err != nil || report.Errors != 1 || report.Entries[0].Error != "target_conflict" || db.updates != 0 || !bytes.Equal(before, objects.stored) {
		t.Fatalf("target conflict=%+v error=%v", report, err)
	}
}
func TestMigrationRejectsOversizedAndInvalidSourceContent(t *testing.T) {
	for name, content := range map[string][]byte{"oversized": bytes.Repeat([]byte("x"), storage.MaxLegacyImageBytes+1), "disguised": []byte("<html>not an image</html>")} {
		t.Run(name, func(t *testing.T) {
			cfg, db, objects := migrationFixture(t)
			cfg.Apply = true
			objects.source = content
			report, err := Run(context.Background(), cfg, db, objects, objects)
			if err != nil || report.Errors != 1 || objects.puts != 0 || db.updates != 0 {
				t.Fatalf("bad content=%+v error=%v", report, err)
			}
		})
	}
}
func TestMigrationApplyRequiresExplicitSourceAllowlist(t *testing.T) {
	cfg, db, objects := migrationFixture(t)
	cfg.Apply = true
	cfg.SourceBuckets = nil
	_, err := Run(context.Background(), cfg, db, objects, objects)
	if err == nil || db.scans != 0 {
		t.Fatalf("apply without allowlist reached database: err=%v scans=%d", err, db.scans)
	}
}

type objectReadFunc func(context.Context, *s3.GetObjectInput) (*s3.GetObjectOutput, error)

func (f objectReadFunc) GetObject(ctx context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return f(ctx, input)
}

type countingObjectBody struct {
	read   int
	closed bool
}

func (b *countingObjectBody) Read(data []byte) (int, error) {
	for i := range data {
		data[i] = 'x'
	}
	b.read += len(data)
	return len(data), nil
}
func (b *countingObjectBody) Close() error { b.closed = true; return nil }

func TestMigrationBoundsStreamingObjectsWithoutTrustingContentLength(t *testing.T) {
	for _, length := range []*int64{nil, aws.Int64(1), aws.Int64(storage.MaxLegacyImageBytes + 1)} {
		body := &countingObjectBody{}
		reader := objectReadFunc(func(context.Context, *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{Body: body, ContentLength: length}, nil
		})
		data, err := readBoundedObject(context.Background(), reader, "legacy-bucket", "question-images/original.png", storage.MaxLegacyImageBytes)
		if !errors.Is(err, storage.ErrImageTooLarge) || data != nil || body.read > storage.MaxLegacyImageBytes+1 || !body.closed {
			t.Fatalf("stream bound: read=%d closed=%v returned=%d error=%v", body.read, body.closed, len(data), err)
		}
		if length != nil && *length > storage.MaxLegacyImageBytes && body.read != 0 {
			t.Error("read body already declared too large")
		}
	}
}

type pagedMigrationDB struct {
	migrationDB
	pages int
	fail  bool
}

func (d *pagedMigrationDB) Scan(_ context.Context, input *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	d.pages++
	if d.fail {
		return nil, errors.New("private database response including image URL")
	}
	if d.pages > 1 && input.ExclusiveStartKey == nil {
		d.t.Error("scan dropped the continuation key")
	}
	return &dynamodb.ScanOutput{LastEvaluatedKey: map[string]ddbtypes.AttributeValue{"id": &ddbtypes.AttributeValueMemberS{Value: "cursor"}}}, nil
}
func TestMigrationScanLimitsAndFailuresAreExplicitAndSanitized(t *testing.T) {
	for _, fail := range []bool{false, true} {
		cfg, _, objects := migrationFixture(t)
		cfg.MaxPages = 2
		db := &pagedMigrationDB{migrationDB: migrationDB{t: t}, fail: fail}
		report, err := Run(context.Background(), cfg, db, objects, objects)
		if err == nil || db.pages > 2 || strings.Contains(err.Error(), "private") || report.Migrated != 0 {
			t.Fatalf("unbounded or disclosed failed scan: report=%+v pages=%d err=%v", report, db.pages, err)
		}
	}
}

func TestMigrationReportsBackendFailuresWithoutURLsOrContent(t *testing.T) {
	cfg, db, objects := migrationFixture(t)
	cfg.Apply = true
	objects.readErr = errors.New("GET https://s3.example?private-secret=token failed with private body")
	report, err := Run(context.Background(), cfg, db, objects, objects)
	encoded, _ := json.Marshal(report)
	if err != nil || report.Errors != 1 || report.Entries[0].Error != "source_read_failed" || bytes.Contains(encoded, []byte("private")) || bytes.Contains(encoded, []byte("https")) {
		t.Fatalf("unsafe report: %s err=%v", encoded, err)
	}
}

func TestMigrationHandlesAnswerSectionImageParents(t *testing.T) {
	cfg, db, objects := migrationFixture(t)
	cfg.Apply = true
	db.item["entity_type"] = &ddbtypes.AttributeValueMemberS{Value: models.EntityTypeCardAnswerSectionImage}
	delete(db.item, "card_id")
	db.item["card_answer_section_id"] = &ddbtypes.AttributeValueMemberS{Value: "section-parent"}
	report, err := Run(context.Background(), cfg, db, objects, objects)
	if err != nil || report.Migrated != 1 || report.Entries[0].EntityType != models.EntityTypeCardAnswerSectionImage {
		t.Fatalf("answer image migration=%+v err=%v", report, err)
	}
}
