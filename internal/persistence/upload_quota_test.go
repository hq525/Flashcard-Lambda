package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/smithy-go"
)

type uploadBudgetHTTPClient func(*http.Request) (*http.Response, error)

func (f uploadBudgetHTTPClient) Do(req *http.Request) (*http.Response, error) { return f(req) }

func uploadBudgetStore(t *testing.T, respond func(map[string]any) (int, string)) *Store {
	t.Helper()
	client := dynamodb.New(dynamodb.Options{
		Region: "ap-southeast-1", Credentials: aws.AnonymousCredentials{}, RetryMaxAttempts: 1,
		BaseEndpoint: aws.String("https://dynamodb.test"),
		HTTPClient: uploadBudgetHTTPClient(func(req *http.Request) (*http.Response, error) {
			if target := req.Header.Get("X-Amz-Target"); target != "DynamoDB_20120810.UpdateItem" {
				t.Errorf("budget requires one atomic UpdateItem, got %s", target)
			}
			var body map[string]any
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			status, response := respond(body)
			return &http.Response{
				StatusCode: status, Header: http.Header{"Content-Type": {"application/x-amz-json-1.0"}},
				Body: io.NopCloser(strings.NewReader(response)), Request: req,
			}, nil
		}),
	})
	return &Store{DB: client, Table: "private-library"}
}

// Decode aliases so assertions cover the database condition/values, without
// depending on the aliases chosen by the application or expression builder.
func expandBudgetExpression(t *testing.T, request map[string]any, field string) string {
	t.Helper()
	expr, _ := request[field].(string)
	names, _ := request["ExpressionAttributeNames"].(map[string]any)
	values, _ := request["ExpressionAttributeValues"].(map[string]any)
	expr = regexp.MustCompile(`#[A-Za-z0-9_]+`).ReplaceAllStringFunc(expr, func(alias string) string {
		name, _ := names[alias].(string)
		return name
	})
	expr = regexp.MustCompile(`:[A-Za-z0-9_]+`).ReplaceAllStringFunc(expr, func(alias string) string {
		value, _ := values[alias].(map[string]any)
		if number, ok := value["N"].(string); ok {
			return number
		}
		text, _ := value["S"].(string)
		return strconv.Quote(text)
	})
	return strings.Join(strings.Fields(expr), " ")
}

// Missing either condition, using <= for count, or comparing bytes to the total
// instead of the remaining allowance would permit requests beyond the budget.
func TestConsumeUploadAtomicallyEnforcesCountAndByteLimits(t *testing.T) {
	calls := 0
	store := uploadBudgetStore(t, func(request map[string]any) (int, string) {
		calls++
		if request["TableName"] != "private-library" || !reflect.DeepEqual(request["Key"], map[string]any{"id": map[string]any{"S": "upload_budget:2026-09-19"}}) {
			t.Errorf("incorrect UTC budget record: %#v", request)
		}
		condition := expandBudgetExpression(t, request, "ConditionExpression")
		wantCondition := `(attribute_not_exists(id) OR entity_type = "upload_budget") AND (attribute_not_exists(uploads) OR uploads < 100) AND (attribute_not_exists(bytes) OR bytes <= 94371840)`
		if condition != wantCondition {
			t.Errorf("unsafe budget condition: %s; want %s", condition, wantCondition)
		}
		update := expandBudgetExpression(t, request, "UpdateExpression")
		if update != `SET entity_type = "upload_budget", expires_at = 1790094600 ADD uploads 1, bytes 10485760` {
			t.Errorf("budget must atomically add count/bytes and set a 72h TTL: %s", update)
		}
		return 200, `{}`
	})
	// Singapore is already the following day; the budget must use UTC instead.
	now := time.Date(2026, time.September, 20, 0, 30, 0, 0, time.FixedZone("SGT", 8*60*60))
	if err := store.consumeUpload(context.Background(), 10*1024*1024, now); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("database calls = %d; want one atomic update", calls)
	}
}

func TestConsumeUploadRejectsInvalidSizesBeforeAWS(t *testing.T) {
	for _, size := range []int64{math.MinInt64, -1, 0, 10*1024*1024 + 1, math.MaxInt64} {
		t.Run(strconv.FormatInt(size, 10), func(t *testing.T) {
			store := uploadBudgetStore(t, func(map[string]any) (int, string) {
				t.Error("invalid upload size reached AWS")
				return 200, `{}`
			})
			if err := store.ConsumeUpload(context.Background(), size); err == nil {
				t.Fatal("invalid upload size was accepted")
			}
		})
	}
}

func TestConsumeUploadReturnsQuotaErrorOnConditionalFailure(t *testing.T) {
	store := uploadBudgetStore(t, func(map[string]any) (int, string) {
		return 400, `{"__type":"ConditionalCheckFailedException","message":"daily budget exhausted"}`
	})
	if err := store.ConsumeUpload(context.Background(), 1); !errors.Is(err, ErrUploadQuota) {
		t.Fatalf("error = %v; want ErrUploadQuota", err)
	}
}

func TestConsumeUploadPreservesServiceErrors(t *testing.T) {
	calls := 0
	store := uploadBudgetStore(t, func(map[string]any) (int, string) {
		calls++
		return 400, `{"__type":"ResourceNotFoundException","message":"missing table"}`
	})
	err := store.ConsumeUpload(context.Background(), 1)
	var apiErr smithy.APIError
	if errors.Is(err, ErrUploadQuota) || !errors.As(err, &apiErr) || apiErr.ErrorCode() != "ResourceNotFoundException" {
		t.Fatalf("service error not preserved: %v", err)
	}
	if calls != 1 {
		t.Fatalf("database calls = %d; failed reservation must not trigger a refund", calls)
	}
}

func TestConsumeUploadStartsNewBudgetAtUTCMidnight(t *testing.T) {
	var ids []string
	store := uploadBudgetStore(t, func(request map[string]any) (int, string) {
		key := request["Key"].(map[string]any)["id"].(map[string]any)["S"].(string)
		ids = append(ids, key)
		return 200, `{}`
	})
	for _, now := range []time.Time{
		time.Date(2026, time.September, 19, 23, 59, 59, 0, time.UTC),
		time.Date(2026, time.September, 20, 0, 0, 0, 0, time.UTC),
	} {
		if err := store.consumeUpload(context.Background(), 1, now); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(ids, []string{"upload_budget:2026-09-19", "upload_budget:2026-09-20"}) {
		t.Fatalf("incorrect UTC daily budget keys: %#v", ids)
	}
}
