package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"flashcard_lambda/internal/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/smithy-go"
)

type typedMutationHTTPClient func(*http.Request) (*http.Response, error)

func (f typedMutationHTTPClient) Do(req *http.Request) (*http.Response, error) { return f(req) }

func typedMutationTestStore(t *testing.T, respond func(string, map[string]any) (int, string)) *Store {
	t.Helper()
	client := dynamodb.New(dynamodb.Options{
		Region: "us-east-1", Credentials: aws.AnonymousCredentials{}, RetryMaxAttempts: 1,
		BaseEndpoint: aws.String("https://dynamodb.test"),
		HTTPClient: typedMutationHTTPClient(func(req *http.Request) (*http.Response, error) {
			var body map[string]any
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			op := strings.TrimPrefix(req.Header.Get("X-Amz-Target"), "DynamoDB_20120810.")
			status, response := respond(op, body)
			return &http.Response{
				StatusCode: status, Header: http.Header{"Content-Type": {"application/x-amz-json-1.0"}},
				Body: io.NopCloser(strings.NewReader(response)), Request: req,
			}, nil
		}),
	})
	return &Store{DB: client, Table: "flashcards"}
}

type typedMutationCase struct {
	name, entityType, operation string
	mutate                      func(*Store, string) (bool, error)
}

func typedEntityMutations[T, C, U any](entityType string, makeRepository func(*Store) Repository[T, C, U], request U) []typedMutationCase {
	return []typedMutationCase{
		{entityType + "/update", entityType, "UpdateItem", func(store *Store, id string) (bool, error) {
			item, err := makeRepository(store).Update(context.Background(), id, request)
			return item != nil, err
		}},
		{entityType + "/delete", entityType, "DeleteItem", func(store *Store, id string) (bool, error) {
			item, err := makeRepository(store).Delete(context.Background(), id)
			return item != nil, err
		}},
	}
}

func genericMutationCases() []typedMutationCase {
	var cases []typedMutationCase
	cases = append(cases, typedEntityMutations("category", NewCategoryRepository, models.UpdateCategoryRequest{Name: "Updated"})...)
	cases = append(cases, typedEntityMutations("tag", NewTagRepository, models.UpdateTagRequest{Name: "Updated"})...)
	cases = append(cases, typedEntityMutations("deck", NewDeckRepository, models.UpdateDeckRequest{Name: "Updated"})...)
	cases = append(cases, typedEntityMutations("card_answer_section", NewCardAnswerSectionRepository, models.UpdateCardAnswerSectionRequest{Answer: "Updated"})...)
	cases = append(cases, typedEntityMutations("card_question_image", NewCardQuestionImageRepository, models.UpdateCardQuestionImageRequest{SequenceNumber: 2})...)
	cases = append(cases, typedEntityMutations("card_answer_section_image", NewCardAnswerSectionImageRepository, models.UpdateCardAnswerSectionImageRequest{SequenceNumber: 2})...)
	// Card edits have their own tested type guard; deletion still uses the generic repository.
	cases = append(cases, typedEntityMutations("card", NewCardRepository, models.UpdateCardRequest{})[1])
	return cases
}

func assertTypedMutationGuard(t *testing.T, req map[string]any, expectedType string) {
	t.Helper()
	condition, _ := req["ConditionExpression"].(string)
	names, _ := req["ExpressionAttributeNames"].(map[string]any)
	condition = regexp.MustCompile(`#[A-Za-z0-9_]+`).ReplaceAllStringFunc(condition, func(alias string) string {
		name, _ := names[alias].(string)
		return name
	})
	condition = strings.ReplaceAll(condition, "exists (", "exists(")
	if !strings.Contains(condition, "attribute_exists(id)") || !strings.Contains(condition, " AND ") {
		t.Errorf("mutation must require existing id AND expected entity type: %q", condition)
	}
	match := regexp.MustCompile(`entity_type = (:[A-Za-z0-9_]+)`).FindStringSubmatch(condition)
	values, _ := req["ExpressionAttributeValues"].(map[string]any)
	if len(match) != 2 || !reflect.DeepEqual(values[match[1]], map[string]any{"S": expectedType}) {
		t.Errorf("mutation is not scoped to %q: condition=%q values=%#v", expectedType, condition, values)
	}
}

func TestGenericMutationsCannotAlterReviewThroughAnotherRepository(t *testing.T) {
	for _, tc := range genericMutationCases() {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			store := typedMutationTestStore(t, func(op string, req map[string]any) (int, string) {
				calls++
				if op != tc.operation || req["TableName"] != "flashcards" || !reflect.DeepEqual(req["Key"], map[string]any{"id": map[string]any{"S": "immutable-review-id"}}) {
					t.Errorf("unexpected mutation target: %s %#v", op, req)
				}
				assertTypedMutationGuard(t, req, tc.entityType)
				// The actual database rejects the condition for entity_type=card_review.
				return 400, `{"__type":"ConditionalCheckFailedException","message":"wrong entity type"}`
			})
			found, err := tc.mutate(store, "immutable-review-id")
			if found || err != nil {
				t.Errorf("wrong-kind mutation = found %v, error %v; want not found without error", found, err)
			}
			if calls != 1 {
				t.Errorf("wrong-kind mutation must not fall back to an unguarded write: calls=%d", calls)
			}
		})
	}
}

func TestGenericMutationsPreserveMatchingEntitySemantics(t *testing.T) {
	for _, tc := range genericMutationCases() {
		t.Run(tc.name, func(t *testing.T) {
			store := typedMutationTestStore(t, func(op string, req map[string]any) (int, string) {
				assertTypedMutationGuard(t, req, tc.entityType)
				wantReturn := "ALL_OLD"
				if tc.operation == "UpdateItem" {
					wantReturn = "ALL_NEW"
				}
				if req["ReturnValues"] != wantReturn {
					t.Errorf("return values=%v; want %s", req["ReturnValues"], wantReturn)
				}
				body, err := json.Marshal(map[string]any{"Attributes": map[string]any{
					"id": map[string]any{"S": "matching-id"}, "entity_type": map[string]any{"S": tc.entityType},
				}})
				if err != nil {
					t.Fatal(err)
				}
				return 200, string(body)
			})
			found, err := tc.mutate(store, "matching-id")
			if !found || err != nil {
				t.Errorf("matching entity mutation = found %v, error %v", found, err)
			}
		})
	}
}

func TestGenericMutationsDistinguishMissingFromBackendFailure(t *testing.T) {
	for _, mutation := range typedEntityMutations("tag", NewTagRepository, models.UpdateTagRequest{Name: "Updated"}) {
		for _, tc := range []struct {
			name, body string
			status     int
			wantError  bool
		}{
			{"missing", `{"__type":"ConditionalCheckFailedException","message":"item absent"}`, 400, false},
			{"service failure", `{"__type":"InternalServerError","message":"unavailable"}`, 500, true},
			{"malformed returned entity", `{"Attributes":{"name":{"M":{}}}}`, 200, true},
		} {
			t.Run(mutation.name+"/"+tc.name, func(t *testing.T) {
				store := typedMutationTestStore(t, func(string, map[string]any) (int, string) { return tc.status, tc.body })
				found, err := mutation.mutate(store, "id")
				if found || (err != nil) != tc.wantError {
					t.Errorf("mutation = found %v, error %v; wantError=%v", found, err, tc.wantError)
				}
				if tc.name == "service failure" {
					var apiErr smithy.APIError
					if !errors.As(err, &apiErr) || apiErr.ErrorCode() != "InternalServerError" {
						t.Errorf("original service failure lost: %v", err)
					}
				}
			})
		}
	}
}
