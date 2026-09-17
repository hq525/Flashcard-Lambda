package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"flashcard_lambda/internal/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

type reviewHTTPClient func(*http.Request) (*http.Response, error)

func (f reviewHTTPClient) Do(req *http.Request) (*http.Response, error) { return f(req) }

// Exercise the real SDK serialization and error decoding without network I/O.
func reviewTestStore(t *testing.T, respond func(string, map[string]any) (int, string)) ReviewStore {
	t.Helper()
	client := dynamodb.New(dynamodb.Options{
		Region: "us-east-1", Credentials: aws.AnonymousCredentials{}, RetryMaxAttempts: 1,
		BaseEndpoint: aws.String("https://dynamodb.test"),
		HTTPClient: reviewHTTPClient(func(req *http.Request) (*http.Response, error) {
			var body map[string]any
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode DynamoDB request: %v", err)
			}
			operation := strings.TrimPrefix(req.Header.Get("X-Amz-Target"), "DynamoDB_20120810.")
			status, response := respond(operation, body)
			return &http.Response{
				StatusCode: status, Header: http.Header{"Content-Type": {"application/x-amz-json-1.0"}},
				Body: io.NopCloser(strings.NewReader(response)), Request: req,
			}, nil
		}),
	})
	return NewReviewStore(&Store{DB: client, Table: "flashcards"})
}

func TestReviewStoreGetCardUsesConsistentReadAndRejectsOtherEntityTypes(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		wantCard       bool
	}{
		{"card", `{"Item":{"id":{"S":"card-1"},"entity_type":{"S":"card"},"question":{"S":"Edited question"}}}`, true},
		{"missing", `{}`, false},
		{"other entity", `{"Item":{"id":{"S":"card-1"},"entity_type":{"S":"tag"}}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := reviewTestStore(t, func(op string, req map[string]any) (int, string) {
				if op != "GetItem" || req["ConsistentRead"] != true || req["TableName"] != "flashcards" {
					t.Errorf("expected consistent GetItem: %s %#v", op, req)
				}
				if !reflect.DeepEqual(req["Key"], map[string]any{"id": map[string]any{"S": "card-1"}}) {
					t.Errorf("wrong card key: %#v", req["Key"])
				}
				return 200, tc.response
			})
			card, err := store.GetCard(context.Background(), "card-1")
			if err != nil || (card != nil) != tc.wantCard {
				t.Fatalf("GetCard = %+v, %v; want card=%v", card, err, tc.wantCard)
			}
			if card != nil && card.Question != "Edited question" {
				t.Errorf("lost question: %+v", card)
			}
		})
	}
}

func TestReviewStoreGetReviewScopesRequestIDAndUsesConsistentRead(t *testing.T) {
	var keys []string
	store := reviewTestStore(t, func(op string, req map[string]any) (int, string) {
		if op != "GetItem" || req["ConsistentRead"] != true {
			t.Errorf("expected consistent GetItem: %s %#v", op, req)
		}
		keys = append(keys, req["Key"].(map[string]any)["id"].(map[string]any)["S"].(string))
		return 200, `{}`
	})
	for _, ids := range [][2]string{{"a:b", "c"}, {"a", "b:c"}, {"a:b", "c"}} {
		review, err := store.GetReview(context.Background(), ids[0], ids[1])
		if err != nil || review != nil {
			t.Fatalf("GetReview = %+v, %v", review, err)
		}
	}
	if keys[0] == keys[1] || keys[0] != keys[2] {
		t.Errorf("review keys must scope requests without ambiguous separators: %v", keys)
	}
}

func TestReviewStoreGetReviewReturnsRecordedSnapshot(t *testing.T) {
	store := reviewTestStore(t, func(op string, req map[string]any) (int, string) {
		return 200, `{"Item":{"id":{"S":"review-1"},"entity_type":{"S":"card_review"},"card_id":{"S":"card-1"},"request_id":{"S":"request-1"},"rating":{"S":"again"},"revision":{"N":"4"},"previous_review_id":{"S":"previous-review"},"reviewed_at":{"S":"2026-09-17T12:00:00.123Z"},"previous_schedule":{"M":{"state":{"S":"review"},"reps":{"N":"3"}}},"schedule":{"M":{"state":{"S":"relearning"},"due_at":{"S":"2026-09-17T12:10:00.123Z"},"reps":{"N":"4"}}}}}`
	})
	review, err := store.GetReview(context.Background(), "card-1", "request-1")
	if err != nil || review == nil {
		t.Fatalf("GetReview = %+v, %v", review, err)
	}
	if review.Rating != "again" || review.Revision != 4 || review.PreviousReviewId != "previous-review" || review.PreviousSchedule.Reps != 3 || review.Schedule.State != "relearning" || review.Schedule.DueAt != "2026-09-17T12:10:00.123Z" {
		t.Errorf("recorded snapshot was changed: %+v", review)
	}
}

func TestReviewStoreConsistentReadsPropagateErrors(t *testing.T) {
	for _, response := range []struct {
		status int
		body   string
	}{
		{500, `{"__type":"InternalServerError","message":"unavailable"}`},
		{200, `{"Item":{"review_revision":{"N":"bad"},"revision":{"N":"bad"}}}`},
	} {
		store := reviewTestStore(t, func(string, map[string]any) (int, string) { return response.status, response.body })
		if card, err := store.GetCard(context.Background(), "card-1"); err == nil || card != nil {
			t.Errorf("GetCard hid read/decoding failure: %+v, %v", card, err)
		}
		if review, err := store.GetReview(context.Background(), "card-1", "request-1"); err == nil || review != nil {
			t.Errorf("GetReview hid read/decoding failure: %+v, %v", review, err)
		}
	}
}

func TestReviewStoreGetReviewRejectsMismatchedIdentity(t *testing.T) {
	store := reviewTestStore(t, func(string, map[string]any) (int, string) {
		return 200, `{"Item":{"entity_type":{"S":"card_review"},"card_id":{"S":"different-card"},"request_id":{"S":"request-1"}}}`
	})
	if review, err := store.GetReview(context.Background(), "card-1", "request-1"); err == nil || review != nil {
		t.Errorf("mismatched review = %+v, %v", review, err)
	}
}

func reviewFixture() models.CardReview {
	return models.CardReview{
		CardId: "card-1", RequestId: "request-1", Rating: "good", Revision: 3,
		PreviousReviewId: "previous-review",
		ReviewedAt:       "2026-09-17T12:00:00Z",
		PreviousSchedule: models.CardSchedule{Version: 1, Algorithm: "fsrs-6", State: "review", Reps: 2},
		Schedule: models.CardSchedule{
			Version: 1, Algorithm: "fsrs-6", DueAt: "2026-09-20T12:00:00Z", LastReviewAt: "2026-09-17T12:00:00Z",
			Stability: 3.5, Difficulty: 4.2, Reps: 3, ScheduledDays: 3, State: "review",
		},
	}
}

func expandReviewExpression(expression string, names map[string]any) string {
	for key, value := range names {
		expression = strings.ReplaceAll(expression, key, value.(string))
	}
	return expression
}

func TestReviewStoreSaveReviewAtomicallyUpdatesOnlySchedulingAttributes(t *testing.T) {
	for _, revision := range []uint64{0, 2} {
		t.Run(fmt.Sprint(revision), func(t *testing.T) {
			calls := 0
			review := reviewFixture()
			review.Revision = revision + 1
			store := reviewTestStore(t, func(op string, req map[string]any) (int, string) {
				calls++
				if op != "TransactWriteItems" {
					t.Fatalf("review and schedule require one transaction; got %s", op)
				}
				items := req["TransactItems"].([]any)
				if len(items) != 2 {
					t.Fatalf("transaction must contain card update and immutable history, got %d actions", len(items))
				}
				update := items[0].(map[string]any)["Update"].(map[string]any)
				put := items[1].(map[string]any)["Put"].(map[string]any)
				if update["TableName"] != "flashcards" || put["TableName"] != "flashcards" {
					t.Error("transaction targets wrong table")
				}
				if !reflect.DeepEqual(update["Key"], map[string]any{"id": map[string]any{"S": "card-1"}}) {
					t.Errorf("transaction updates wrong card: %#v", update["Key"])
				}
				names := update["ExpressionAttributeNames"].(map[string]any)
				expr := expandReviewExpression(update["UpdateExpression"].(string), names)
				assignments := strings.Split(strings.TrimPrefix(expr, "SET "), ",")
				want := map[string]bool{"schedule": true, "review_revision": true, "last_accessed_date_time": true, "previously_correct": true, "updated_date_time": true, "last_review_id": true}
				for _, assignment := range assignments {
					attribute := strings.TrimSpace(strings.SplitN(assignment, "=", 2)[0])
					if !want[attribute] {
						t.Errorf("review overwrites unrelated attribute %q", attribute)
					}
					delete(want, attribute)
				}
				if len(want) != 0 {
					t.Errorf("missing scheduling updates: %v", want)
				}
				values := update["ExpressionAttributeValues"].(map[string]any)
				for attribute, expected := range map[string]any{
					"review_revision":         map[string]any{"N": fmt.Sprint(revision + 1)},
					"last_accessed_date_time": map[string]any{"S": "2026-09-17T12:00:00Z"},
					"updated_date_time":       map[string]any{"S": "2026-09-17T12:00:00Z"},
					"previously_correct":      map[string]any{"BOOL": true},
					"last_review_id":          map[string]any{"S": ReviewID("card-1", "request-1")},
				} {
					match := regexp.MustCompile(attribute + ` = (:[a-zA-Z0-9_]+)`).FindStringSubmatch(expr)
					if len(match) != 2 || !reflect.DeepEqual(values[match[1]], expected) {
						t.Errorf("incorrect %s in update %s, values=%#v", attribute, expr, values)
					}
				}
				condition := expandReviewExpression(update["ConditionExpression"].(string), names)
				for _, required := range []string{"attribute_exists(id)", "entity_type =", "review_revision =", "attribute_not_exists(review_deleting)"} {
					if !strings.Contains(condition, required) {
						t.Errorf("missing guard %q in %q", required, condition)
					}
				}
				if strings.Contains(condition, "attribute_not_exists(review_revision)") != (revision == 0) {
					t.Errorf("missing revision allowed only for legacy revision zero: %s", condition)
				}
				for attribute, expected := range map[string]any{
					"review_revision": map[string]any{"N": fmt.Sprint(revision)},
					"entity_type":     map[string]any{"S": "card"},
				} {
					match := regexp.MustCompile(attribute + ` = (:[a-zA-Z0-9_]+)`).FindStringSubmatch(condition)
					if len(match) != 2 || !reflect.DeepEqual(values[match[1]], expected) {
						t.Errorf("incorrect %s guard in %s, values=%#v", attribute, condition, values)
					}
				}
				putCondition := expandReviewExpression(put["ConditionExpression"].(string), put["ExpressionAttributeNames"].(map[string]any))
				if putCondition != "attribute_not_exists(id)" {
					t.Errorf("history must not be replaced: %s", putCondition)
				}
				item := put["Item"].(map[string]any)
				for name, value := range map[string]string{"id": ReviewID("card-1", "request-1"), "entity_type": "card_review", "card_id": "card-1", "request_id": "request-1", "rating": "good", "reviewed_at": "2026-09-17T12:00:00Z"} {
					if !reflect.DeepEqual(item[name], map[string]any{"S": value}) {
						t.Errorf("history %s = %#v; want %s", name, item[name], value)
					}
				}
				if item["previous_schedule"] == nil || item["schedule"] == nil {
					t.Error("review must retain previous and resulting schedule snapshots")
				}
				if !reflect.DeepEqual(item["previous_review_id"], map[string]any{"S": "previous-review"}) {
					t.Errorf("review must retain its predecessor for consistent cleanup: %#v", item)
				}
				scheduleMatch := regexp.MustCompile(`schedule = (:[a-zA-Z0-9_]+)`).FindStringSubmatch(expr)
				if len(scheduleMatch) != 2 || !reflect.DeepEqual(values[scheduleMatch[1]], item["schedule"]) {
					t.Error("card schedule and immutable history snapshot differ")
				}
				return 200, `{}`
			})
			if err := store.SaveReview(context.Background(), "card-1", revision, review); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Errorf("save made %d calls; want a single transaction", calls)
			}
		})
	}
}

func TestReviewStoreSaveAgainRecordsFailedRecall(t *testing.T) {
	store := reviewTestStore(t, func(op string, req map[string]any) (int, string) {
		update := req["TransactItems"].([]any)[0].(map[string]any)["Update"].(map[string]any)
		expr := expandReviewExpression(update["UpdateExpression"].(string), update["ExpressionAttributeNames"].(map[string]any))
		match := regexp.MustCompile(`previously_correct = (:[a-zA-Z0-9_]+)`).FindStringSubmatch(expr)
		values := update["ExpressionAttributeValues"].(map[string]any)
		if len(match) != 2 || !reflect.DeepEqual(values[match[1]], map[string]any{"BOOL": false}) {
			t.Errorf("Again must record failed recall: %#v", update)
		}
		return 200, `{}`
	})
	review := reviewFixture()
	review.Rating = "again"
	if err := store.SaveReview(context.Background(), "card-1", 2, review); err != nil {
		t.Fatal(err)
	}
}

func TestReviewStoreListReviewsPaginatesFilteredIndexAndSortsChronologically(t *testing.T) {
	pages := 0
	store := reviewTestStore(t, func(op string, req map[string]any) (int, string) {
		pages++
		if op != "Query" || req["TableName"] != "flashcards" || req["IndexName"] != "card_id-index" {
			t.Fatalf("expected card_id-index query, got %s %#v", op, req)
		}
		names := req["ExpressionAttributeNames"].(map[string]any)
		if !strings.Contains(expandReviewExpression(req["KeyConditionExpression"].(string), names), "card_id =") ||
			!strings.Contains(expandReviewExpression(req["FilterExpression"].(string), names), "entity_type =") {
			t.Errorf("missing card key / entity type filter: %#v", req)
		}
		values, _ := json.Marshal(req["ExpressionAttributeValues"])
		if !strings.Contains(string(values), `"card-1"`) || !strings.Contains(string(values), `"card_review"`) {
			t.Errorf("wrong card or entity query values: %s", values)
		}
		switch pages {
		case 1:
			return 200, `{"Items":[],"LastEvaluatedKey":{"id":{"S":"filtered-out-image"}}}`
		case 2:
			if !reflect.DeepEqual(req["ExclusiveStartKey"], map[string]any{"id": map[string]any{"S": "filtered-out-image"}}) {
				t.Errorf("did not continue past filtered empty page: %#v", req)
			}
			return 200, `{"Items":[
				{"id":{"S":"later-fraction"},"entity_type":{"S":"card_review"},"card_id":{"S":"card-1"},"reviewed_at":{"S":"2026-09-17T12:00:00.5Z"},"revision":{"N":"3"}},
				{"id":{"S":"same-time-second"},"entity_type":{"S":"card_review"},"card_id":{"S":"card-1"},"reviewed_at":{"S":"2026-09-17T12:00:00Z"},"revision":{"N":"2"}},
				{"id":{"S":"same-time-first"},"entity_type":{"S":"card_review"},"card_id":{"S":"card-1"},"reviewed_at":{"S":"2026-09-17T12:00:00Z"},"revision":{"N":"1"}}
			],"LastEvaluatedKey":{}}`
		default:
			t.Fatalf("continued after empty pagination key: %d requests", pages)
			return 500, `{}`
		}
	})
	reviews, err := store.ListReviews(context.Background(), "card-1")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, review := range reviews {
		ids = append(ids, review.Id)
	}
	if !reflect.DeepEqual(ids, []string{"same-time-first", "same-time-second", "later-fraction"}) || pages != 2 {
		t.Errorf("history = %v after %d pages", ids, pages)
	}
}

func TestReviewStoreListReviewsDoesNotReturnPartialOrMalformedHistory(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		status         int
	}{
		{"read failure", `{"__type":"InternalServerError","message":"unavailable"}`, 500},
		{"invalid revision", `{"Items":[{"id":{"S":"broken"},"revision":{"N":"not-an-integer"}}]}`, 200},
		{"invalid timestamp", `{"Items":[{"id":{"S":"broken"},"reviewed_at":{"S":"yesterday"}}]}`, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			store := reviewTestStore(t, func(op string, req map[string]any) (int, string) {
				calls++
				if calls == 1 {
					return 200, `{"Items":[{"id":{"S":"first"},"reviewed_at":{"S":"2026-09-17T12:00:00Z"}}],"LastEvaluatedKey":{"id":{"S":"first"}}}`
				}
				return tc.status, tc.response
			})
			reviews, err := store.ListReviews(context.Background(), "card-1")
			if err == nil || reviews != nil {
				t.Errorf("must surface failure instead of partial history: reviews=%+v error=%v", reviews, err)
			}
		})
	}
}

func TestReviewStoreListReviewsReturnsEmptyArray(t *testing.T) {
	store := reviewTestStore(t, func(string, map[string]any) (int, string) { return 200, `{"Items":[]}` })
	reviews, err := store.ListReviews(context.Background(), "card-1")
	if err != nil || reviews == nil || len(reviews) != 0 {
		t.Errorf("empty history = %#v, %v; want non-nil empty slice", reviews, err)
	}
}

func TestReviewStoreDeleteReviewsFollowsConsistentHistoryAndResumesAfterFailure(t *testing.T) {
	head := "review-2"
	marked, reads, transactions := 0, 0, 0
	failed := false
	var deleted []string
	store := reviewTestStore(t, func(op string, req map[string]any) (int, string) {
		switch op {
		case "UpdateItem":
			marked++
			names := req["ExpressionAttributeNames"].(map[string]any)
			update := expandReviewExpression(req["UpdateExpression"].(string), names)
			condition := expandReviewExpression(req["ConditionExpression"].(string), names)
			if !strings.Contains(update, "review_deleting =") || !strings.Contains(condition, "attribute_exists(id)") || !strings.Contains(condition, "entity_type =") || req["ReturnValues"] != "ALL_NEW" {
				t.Errorf("cleanup must mark existing card and return latest history pointer: %#v", req)
			}
			return 200, fmt.Sprintf(`{"Attributes":{"id":{"S":"card-1"},"entity_type":{"S":"card"},"last_review_id":{"S":%q},"review_deleting":{"BOOL":true}}}`, head)
		case "GetItem":
			reads++
			if marked == 0 || req["ConsistentRead"] != true || !reflect.DeepEqual(req["Key"], map[string]any{"id": map[string]any{"S": head}}) {
				t.Errorf("cleanup must strongly read the current head after marking: %#v", req)
			}
			previous := ""
			if head == "review-2" {
				previous = "review-1"
			}
			return 200, fmt.Sprintf(`{"Item":{"id":{"S":%q},"card_id":{"S":"card-1"},"entity_type":{"S":"card_review"},"previous_review_id":{"S":%q}}}`, head, previous)
		case "TransactWriteItems":
			transactions++
			items := req["TransactItems"].([]any)
			if len(items) != 2 {
				t.Fatalf("cleanup must atomically advance pointer and delete log: %#v", req)
			}
			update := items[0].(map[string]any)["Update"].(map[string]any)
			del := items[1].(map[string]any)["Delete"].(map[string]any)
			if !reflect.DeepEqual(del["Key"], map[string]any{"id": map[string]any{"S": head}}) {
				t.Errorf("cleanup deletes wrong history item: %#v", del)
			}
			names := update["ExpressionAttributeNames"].(map[string]any)
			condition := expandReviewExpression(update["ConditionExpression"].(string), names)
			for _, guard := range []string{"last_review_id =", "review_deleting =", "entity_type ="} {
				if !strings.Contains(condition, guard) {
					t.Errorf("missing cleanup guard %q: %s", guard, condition)
				}
			}
			values := update["ExpressionAttributeValues"].(map[string]any)
			match := regexp.MustCompile(`last_review_id = (:[a-zA-Z0-9_]+)`).FindStringSubmatch(condition)
			if len(match) != 2 || !reflect.DeepEqual(values[match[1]], map[string]any{"S": head}) {
				t.Errorf("cleanup must compare current head: %#v", update)
			}
			updateExpression := expandReviewExpression(update["UpdateExpression"].(string), names)
			if head == "review-2" {
				match := regexp.MustCompile(`last_review_id = (:[a-zA-Z0-9_]+)`).FindStringSubmatch(updateExpression)
				if len(match) != 2 || !reflect.DeepEqual(values[match[1]], map[string]any{"S": "review-1"}) {
					t.Errorf("cleanup must persist predecessor before retry: %#v", update)
				}
				deleted = append(deleted, head)
				head = "review-1"
				return 200, `{}`
			}
			if updateExpression != "REMOVE last_review_id" {
				t.Errorf("last history deletion must remove pointer: %s", updateExpression)
			}
			if !failed {
				failed = true
				return 500, `{"__type":"InternalServerError","message":"temporary failure"}`
			}
			deleted = append(deleted, head)
			head = ""
			return 200, `{}`
		default:
			t.Fatalf("cleanup must not rely on eventual index or separate deletes: %s", op)
			return 500, `{}`
		}
	})
	if err := store.DeleteReviews(context.Background(), "card-1"); err == nil {
		t.Fatal("must report failed transaction")
	}
	if head != "review-1" || !reflect.DeepEqual(deleted, []string{"review-2"}) {
		t.Fatalf("unexpected partial state: head=%s deleted=%v", head, deleted)
	}
	if err := store.DeleteReviews(context.Background(), "card-1"); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteReviews(context.Background(), "card-1"); err != nil {
		t.Fatal("empty cleanup is idempotent:", err)
	}
	if head != "" || marked != 3 || reads != 3 || transactions != 3 || !reflect.DeepEqual(deleted, []string{"review-2", "review-1"}) {
		t.Errorf("cleanup did not resume safely: head=%s marks=%d reads=%d transactions=%d deleted=%v", head, marked, reads, transactions, deleted)
	}
}

func TestReviewStoreDeleteReviewsPropagatesErrorsWithoutSkippingHistory(t *testing.T) {
	for _, tc := range []struct {
		name, operation, response string
		status                    int
		wantError                 bool
	}{
		{"mark failure", "UpdateItem", `{"__type":"InternalServerError"}`, 500, true},
		{"missing card", "UpdateItem", `{"__type":"ConditionalCheckFailedException"}`, 400, false},
		{"history read failure", "GetItem", `{"__type":"InternalServerError"}`, 500, true},
		{"missing pointed review", "GetItem", `{}`, 200, true},
		{"different card review", "GetItem", `{"Item":{"id":{"S":"review-1"},"card_id":{"S":"other-card"},"entity_type":{"S":"card_review"}}}`, 200, true},
		{"different entity", "GetItem", `{"Item":{"id":{"S":"review-1"},"card_id":{"S":"card-1"},"entity_type":{"S":"card_question_image"}}}`, 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := reviewTestStore(t, func(op string, req map[string]any) (int, string) {
				if op == tc.operation {
					return tc.status, tc.response
				}
				if op != "UpdateItem" {
					t.Fatalf("must not discard or skip failed history: %s", op)
				}
				return 200, `{"Attributes":{"id":{"S":"card-1"},"entity_type":{"S":"card"},"last_review_id":{"S":"review-1"}}}`
			})
			if err := store.DeleteReviews(context.Background(), "card-1"); (err != nil) != tc.wantError {
				t.Errorf("cleanup error = %v; wantError=%v", err, tc.wantError)
			}
		})
	}
}

func TestReviewStoreSaveReviewReportsConflictsAndWriteFailures(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		conflict       bool
	}{
		{"stale revision", `{"__type":"TransactionCanceledException","CancellationReasons":[{"Code":"ConditionalCheckFailed"},{"Code":"None"}]}`, true},
		{"duplicate request", `{"__type":"TransactionCanceledException","CancellationReasons":[{"Code":"None"},{"Code":"ConditionalCheckFailed"}]}`, true},
		{"concurrent transaction", `{"__type":"TransactionConflictException","message":"busy"}`, true},
		{"invalid transaction", `{"__type":"TransactionCanceledException","CancellationReasons":[{"Code":"ValidationError"}]}`, false},
		{"service failure", `{"__type":"InternalServerError","message":"unavailable"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			store := reviewTestStore(t, func(op string, req map[string]any) (int, string) {
				calls++
				if op != "TransactWriteItems" {
					t.Errorf("unexpected independent history/card write: %s", op)
				}
				return 400, tc.response
			})
			err := store.SaveReview(context.Background(), "card-1", 2, reviewFixture())
			if err == nil || errors.Is(err, ErrReviewConflict) != tc.conflict {
				t.Errorf("SaveReview error = %v; want conflict=%v", err, tc.conflict)
			}
			if calls != 1 {
				t.Errorf("failed transaction made %d calls; history must never be written separately", calls)
			}
		})
	}
}
