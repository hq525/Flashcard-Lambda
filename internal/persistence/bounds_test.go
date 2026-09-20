package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"flashcard_lambda/internal/models"
)

func TestRepositoryGetRejectsStoredEntityTypeBeforeUnmarshalling(t *testing.T) {
	for _, entityType := range []string{`{"S":"card_review"}`, `{"N":"1"}`, `{"NULL":true}`} {
		t.Run(entityType, func(t *testing.T) {
			store := typedMutationTestStore(t, func(op string, req map[string]any) (int, string) {
				if op != "GetItem" {
					t.Fatalf("unexpected operation %s", op)
				}
				// A malformed name must not get decoded when the type is wrong.
				return 200, `{"Item":{"id":{"S":"shared-id"},"entity_type":` + entityType + `,"name":{"M":{}}}}`
			})
			item, err := NewCategoryRepository(store).Get(context.Background(), "shared-id")
			if item != nil || err != nil {
				t.Fatalf("wrong-type Get returned item=%+v err=%v; want absent", item, err)
			}
		})
	}
}

func TestRepositoryGetMatchingTypeAndMissingItem(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		want           bool
	}{
		{"matching", `{"Item":{"id":{"S":"category-1"},"entity_type":{"S":"category"},"name":{"S":"kept"}}}`, true},
		{"missing", `{}`, false},
		{"empty", `{"Item":{}}`, false},
		{"missing discriminator", `{"Item":{"id":{"S":"category-1"},"name":{"S":"kept"}}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := typedMutationTestStore(t, func(string, map[string]any) (int, string) { return 200, tc.response })
			item, err := NewCategoryRepository(store).Get(context.Background(), "category-1")
			if err != nil || (item != nil) != tc.want {
				t.Fatalf("Get=%+v,%v; want present=%v", item, err, tc.want)
			}
			if tc.want && item.Name != "kept" {
				t.Errorf("matching entity content lost: %+v", item)
			}
		})
	}
}

func TestRepositoryGetUsesStrongConsistencyForNewImageParents(t *testing.T) {
	for _, tc := range []struct {
		entityType string
		get        func(*Store) (bool, error)
	}{
		{models.EntityTypeCard, func(store *Store) (bool, error) {
			item, err := NewCardRepository(store).Get(context.Background(), "new-parent")
			return item != nil, err
		}},
		{models.EntityTypeCardAnswerSection, func(store *Store) (bool, error) {
			item, err := NewCardAnswerSectionRepository(store).Get(context.Background(), "new-parent")
			return item != nil, err
		}},
	} {
		t.Run(tc.entityType, func(t *testing.T) {
			store := typedMutationTestStore(t, func(op string, req map[string]any) (int, string) {
				if op != "GetItem" {
					t.Fatalf("unexpected operation %s", op)
				}
				if req["ConsistentRead"] != true {
					t.Error("typed parent lookup must request a strongly consistent GetItem")
					// An eventual read can miss a parent created just before upload.
					return 200, `{}`
				}
				return 200, `{"Item":{"id":{"S":"new-parent"},"entity_type":{"S":"` + tc.entityType + `"}}}`
			})
			found, err := tc.get(store)
			if err != nil || !found {
				t.Fatalf("newly created parent lookup: found=%v err=%v", found, err)
			}
		})
	}
}

func TestQueryResultsStopAtExplicitItemPageAndByteLimits(t *testing.T) {
	for _, tc := range []struct {
		name                             string
		count, pages, textSize, maxCalls int
	}{
		{"item count", 100, 11, 0, 11},
		{"filtered empty pages", 0, 51, 0, 50},
		{"response bytes", 1, 20, 300000, 14},
	} {
		for _, reviews := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reviews=%v", tc.name, reviews), func(t *testing.T) {
				calls := 0
				store := typedMutationTestStore(t, func(op string, req map[string]any) (int, string) {
					calls++
					if op != "Query" {
						t.Fatalf("unexpected operation %s", op)
					}
					limit, ok := req["Limit"].(float64)
					if !ok || limit < 1 || limit > 100 {
						t.Errorf("query must bound evaluated items to 100; got %v", req["Limit"])
					}
					entries := make([]any, 0, tc.count)
					count := tc.count
					if ok && count > int(limit) {
						count = int(limit)
					}
					for i := 0; i < count; i++ {
						entries = append(entries, map[string]any{
							"id":          map[string]any{"S": fmt.Sprintf("entry-%d-%d", calls, i)},
							"name":        map[string]any{"S": strings.Repeat("x", tc.textSize)},
							"request_id":  map[string]any{"S": strings.Repeat("x", tc.textSize)},
							"reviewed_at": map[string]any{"S": "2026-09-20T00:00:00Z"},
						})
					}
					body := map[string]any{"Items": entries}
					if calls < tc.pages {
						body["LastEvaluatedKey"] = map[string]any{"id": map[string]any{"S": fmt.Sprintf("cursor-%d", calls)}}
					}
					encoded, err := json.Marshal(body)
					if err != nil {
						t.Fatal(err)
					}
					return 200, string(encoded)
				})
				var err error
				var length int
				if reviews {
					var items []models.CardReview
					items, err = NewReviewStore(store).ListReviews(context.Background(), "card")
					length = len(items)
				} else {
					var items []models.Category
					items, err = QueryIndex[models.Category](context.Background(), store, IndexEntityType, "entity_type", "category", "")
					length = len(items)
				}
				if !errors.Is(err, ErrResultLimit) || length != 0 {
					t.Errorf("limit returned %d partial items, error=%v; want error with no items", length, err)
				}
				if calls > tc.maxCalls {
					t.Errorf("query made %d calls; maximum %d", calls, tc.maxCalls)
				}
			})
		}
	}
}

func TestQueryIndexReturnsCompleteResultsAtItemLimit(t *testing.T) {
	calls := 0
	store := typedMutationTestStore(t, func(_ string, req map[string]any) (int, string) {
		calls++
		if calls > 1 && req["ExclusiveStartKey"] == nil {
			t.Error("next page omitted cursor")
		}
		entries := make([]any, 100)
		for i := range entries {
			entries[i] = map[string]any{"id": map[string]any{"S": fmt.Sprintf("item-%d-%d", calls, i)}}
		}
		body := map[string]any{"Items": entries}
		if calls < 10 {
			body["LastEvaluatedKey"] = map[string]any{"id": map[string]any{"S": "cursor"}}
		}
		encoded, _ := json.Marshal(body)
		return 200, string(encoded)
	})
	items, err := QueryIndex[models.Category](context.Background(), store, IndexEntityType, "entity_type", "category", "")
	if err != nil || len(items) != 1000 || calls != 10 {
		t.Fatalf("query returned items=%d calls=%d error=%v", len(items), calls, err)
	}
	if items[0].Id != "item-1-0" || items[999].Id != "item-10-99" {
		t.Error("query lost page contents")
	}
}
