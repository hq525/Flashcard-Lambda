package persistence

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"flashcard_lambda/internal/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

type cardUpdateHTTPClient func(*http.Request) (*http.Response, error)

func (f cardUpdateHTTPClient) Do(req *http.Request) (*http.Response, error) { return f(req) }

func cardUpdateTestRepository(t *testing.T, respond func(map[string]any) (int, string)) Repository[models.Card, models.CreateCardRequest, models.UpdateCardRequest] {
	t.Helper()
	client := dynamodb.New(dynamodb.Options{
		Region: "us-east-1", Credentials: aws.AnonymousCredentials{}, RetryMaxAttempts: 1,
		BaseEndpoint: aws.String("https://dynamodb.test"),
		HTTPClient: cardUpdateHTTPClient(func(req *http.Request) (*http.Response, error) {
			if target := req.Header.Get("X-Amz-Target"); target != "DynamoDB_20120810.UpdateItem" {
				t.Fatalf("card update should use conditional UpdateItem, got %s", target)
			}
			var body map[string]any
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			status, response := respond(body)
			return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/x-amz-json-1.0"}}, Body: io.NopCloser(strings.NewReader(response)), Request: req}, nil
		}),
	})
	return NewCardRepository(&Store{DB: client, Table: "flashcards"})
}

func staleCardEdit() models.UpdateCardRequest {
	return models.UpdateCardRequest{Question: "Edited question", TagIds: []string{"new-tag"}, PreviouslyCorrect: false, LastAccessedDateTime: "2026-09-01T00:00:00Z", LeitnerBox: 1}
}

func checkCardUpdateWire(t *testing.T, req map[string]any, migrated bool) {
	t.Helper()
	if req["TableName"] != "flashcards" || req["ReturnValues"] != "ALL_NEW" || !reflect.DeepEqual(req["Key"], map[string]any{"id": map[string]any{"S": "card-1"}}) {
		t.Errorf("wrong update target or return values: %#v", req)
	}
	names := req["ExpressionAttributeNames"].(map[string]any)
	values := req["ExpressionAttributeValues"].(map[string]any)
	expand := func(expr string) string {
		return regexp.MustCompile(`#[A-Za-z0-9_]+`).ReplaceAllStringFunc(expr, func(alias string) string { return names[alias].(string) })
	}
	condition := strings.ReplaceAll(expand(req["ConditionExpression"].(string)), "exists (", "exists(")
	wantSchedule := "attribute_not_exists(schedule)"
	if migrated {
		wantSchedule = "attribute_exists(schedule)"
	}
	for _, required := range []string{"attribute_exists(id)", "entity_type =", wantSchedule} {
		if !strings.Contains(condition, required) {
			t.Errorf("missing guard %q in %q", required, condition)
		}
	}
	typeMatch := regexp.MustCompile(`entity_type = (:[A-Za-z0-9_]+)`).FindStringSubmatch(condition)
	if len(typeMatch) != 2 || !reflect.DeepEqual(values[typeMatch[1]], map[string]any{"S": "card"}) {
		t.Errorf("update not scoped to card entity: %s, %#v", condition, values)
	}
	writes := map[string]any{}
	update := expand(req["UpdateExpression"].(string))
	for _, assignment := range strings.Split(strings.TrimSpace(strings.TrimPrefix(update, "SET ")), ",") {
		parts := strings.SplitN(assignment, "=", 2)
		if len(parts) != 2 {
			t.Fatalf("unexpected update assignment %q", assignment)
		}
		writes[strings.TrimSpace(parts[0])] = values[strings.TrimSpace(parts[1])]
	}
	want := map[string]any{
		"question": map[string]any{"S": "Edited question"},
		"tag_ids":  map[string]any{"L": []any{map[string]any{"S": "new-tag"}}},
	}
	if !migrated {
		want["previously_correct"] = map[string]any{"BOOL": false}
		want["last_accessed_date_time"] = map[string]any{"S": "2026-09-01T00:00:00Z"}
		want["leitner_box"] = map[string]any{"N": "1"}
	}
	updated, ok := writes["updated_date_time"].(map[string]any)
	if !ok {
		t.Fatalf("missing edit timestamp: %#v", writes)
	}
	if _, err := time.Parse(time.RFC3339, updated["S"].(string)); err != nil {
		t.Errorf("bad updated timestamp: %v", err)
	}
	delete(writes, "updated_date_time")
	if !reflect.DeepEqual(writes, want) {
		t.Errorf("wrong edited attributes (migrated=%v): got %#v, want %#v", migrated, writes, want)
	}
}

const cardConditionalFailure = `{"__type":"com.amazonaws.dynamodb.v20120810#ConditionalCheckFailedException","message":"condition failed"}`

const migratedCardResponse = `{"Attributes":{"id":{"S":"card-1"},"entity_type":{"S":"card"},"question":{"S":"Edited question"},"tag_ids":{"L":[{"S":"new-tag"}]},"previously_correct":{"BOOL":true},"last_accessed_date_time":{"S":"2026-09-17T12:00:00Z"},"leitner_box":{"N":"5"},"review_revision":{"N":"9"},"schedule":{"M":{"version":{"N":"1"},"algorithm":{"S":"fsrs-6"},"due_at":{"S":"2026-10-17T12:00:00Z"},"state":{"S":"review"},"reps":{"N":"9"}}}}}`

func TestMigratedCardEditWritesOnlyContentAndReturnsCurrentReviewState(t *testing.T) {
	calls := 0
	repo := cardUpdateTestRepository(t, func(req map[string]any) (int, string) {
		calls++
		checkCardUpdateWire(t, req, true)
		return 200, migratedCardResponse
	})
	card, err := repo.Update(context.Background(), "card-1", staleCardEdit())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || card == nil {
		t.Fatalf("Update = %+v; calls=%d", card, calls)
	}
	if card.Question != "Edited question" || !reflect.DeepEqual(card.TagIds, []string{"new-tag"}) || !card.PreviouslyCorrect || card.LastAccessedDateTime != "2026-09-17T12:00:00Z" || card.LeitnerBox != 5 || card.ReviewRevision != 9 || card.Schedule == nil || card.Schedule.DueAt != "2026-10-17T12:00:00Z" {
		t.Errorf("lost content or current scheduling state: %+v", card)
	}
}

func TestLegacyCardEditRetainsLegacySchedulingFields(t *testing.T) {
	calls := 0
	repo := cardUpdateTestRepository(t, func(req map[string]any) (int, string) {
		calls++
		checkCardUpdateWire(t, req, calls == 1)
		if calls == 1 {
			return 400, cardConditionalFailure
		}
		return 200, `{"Attributes":{"id":{"S":"card-1"},"entity_type":{"S":"card"},"question":{"S":"Edited question"},"previously_correct":{"BOOL":false},"last_accessed_date_time":{"S":"2026-09-01T00:00:00Z"},"leitner_box":{"N":"1"}}}`
	})
	card, err := repo.Update(context.Background(), "card-1", staleCardEdit())
	if err != nil || card == nil || calls != 2 {
		t.Fatalf("Update = %+v, %v; calls=%d", card, err, calls)
	}
	if card.LastAccessedDateTime != "2026-09-01T00:00:00Z" || card.LeitnerBox != 1 || card.Schedule != nil {
		t.Errorf("legacy update lost: %+v", card)
	}
}

func TestCardEditRetriesContentOnlyWhenFirstReviewRacesLegacyWrite(t *testing.T) {
	calls := 0
	repo := cardUpdateTestRepository(t, func(req map[string]any) (int, string) {
		calls++
		checkCardUpdateWire(t, req, calls != 2)
		// The card starts legacy. Its first review creates schedule before the
		// legacy update reaches DynamoDB, so both initial conditions fail.
		if calls <= 2 {
			return 400, cardConditionalFailure
		}
		return 200, migratedCardResponse
	})
	card, err := repo.Update(context.Background(), "card-1", staleCardEdit())
	if err != nil || card == nil || calls != 3 {
		t.Fatalf("Update = %+v, %v; calls=%d", card, err, calls)
	}
	if !card.PreviouslyCorrect || card.ReviewRevision != 9 || card.Schedule == nil {
		t.Errorf("racing review overwritten: %+v", card)
	}
}

func TestMissingCardEditDoesNotUpsertOrRetryForever(t *testing.T) {
	calls := 0
	repo := cardUpdateTestRepository(t, func(req map[string]any) (int, string) {
		calls++
		if calls > 3 {
			t.Fatal("unbounded missing-card retry")
		}
		checkCardUpdateWire(t, req, calls != 2)
		return 400, cardConditionalFailure
	})
	card, err := repo.Update(context.Background(), "card-1", staleCardEdit())
	if err != nil || card != nil || calls != 3 {
		t.Fatalf("Update = %+v, %v; calls=%d", card, err, calls)
	}
}

func TestCardEditPropagatesServiceErrorsWithoutUnsafeFallback(t *testing.T) {
	for _, failAt := range []int{1, 2, 3} {
		calls := 0
		repo := cardUpdateTestRepository(t, func(req map[string]any) (int, string) {
			calls++
			if calls < failAt {
				return 400, cardConditionalFailure
			}
			return 500, `{"__type":"com.amazonaws.dynamodb.v20120810#InternalServerError","message":"unavailable"}`
		})
		card, err := repo.Update(context.Background(), "card-1", staleCardEdit())
		if err == nil || card != nil || calls != failAt {
			t.Errorf("stage %d: Update = %+v, %v; calls=%d", failAt, card, err, calls)
		}
	}
}

func TestCardEditPropagatesReturnedCardDecodeError(t *testing.T) {
	repo := cardUpdateTestRepository(t, func(req map[string]any) (int, string) {
		return 200, `{"Attributes":{"review_revision":{"S":"not-a-number"}}}`
	})
	if card, err := repo.Update(context.Background(), "card-1", staleCardEdit()); err == nil || card != nil {
		t.Fatalf("Update = %+v, %v", card, err)
	}
}
