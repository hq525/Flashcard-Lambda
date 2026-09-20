package httpapi

import (
	"context"
	"flashcard_lambda/internal/persistence"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flashcard_lambda/internal/models"
)

// These requests must fail before the repository receives a mutation.
func TestCRUDRejectsAmbiguousAndOversizedJSON(t *testing.T) {
	for _, operation := range []string{"create", "update"} {
		for _, tc := range []struct {
			name, body string
			status     int
		}{
			{"unknown field", `{"name":"valid","secret":"do-not-echo"}`, 422},
			{"trailing object", `{"name":"valid"} {"secret":"do-not-echo"}`, 422},
			{"trailing token", `{"name":"valid"} true`, 422},
			{"oversized value", `{"name":"` + strings.Repeat("x", 128*1024) + `"}`, 413},
			{"oversized trailing whitespace", `{"name":"valid"}` + strings.Repeat(" ", 128*1024), 413},
		} {
			t.Run(operation+"/"+tc.name, func(t *testing.T) {
				calls := 0
				repo := &boundedRequestRepo[models.Category, models.CreateCategoryRequest, models.UpdateCategoryRequest]{
					CreateFn: func(context.Context, models.CreateCategoryRequest) (*models.Category, error) {
						calls++
						return &models.Category{}, nil
					},
					UpdateFn: func(context.Context, string, models.UpdateCategoryRequest) (*models.Category, error) {
						calls++
						return &models.Category{}, nil
					},
				}
				resource := Resource[models.Category, models.CreateCategoryRequest, models.UpdateCategoryRequest]{Repo: repo}
				request := httptest.NewRequest("POST", "/category?id=category-1", strings.NewReader(tc.body))
				request.ContentLength = -1 // Bound the reader even for chunked requests.
				response := httptest.NewRecorder()
				if operation == "create" {
					resource.Create(response, request)
				} else {
					resource.Update(response, request)
				}
				if response.Code != tc.status || calls != 0 {
					t.Errorf("status=%d calls=%d; want status=%d and no mutation", response.Code, calls, tc.status)
				}
				if strings.Contains(response.Body.String(), "do-not-echo") {
					t.Error("error disclosed request content")
				}
			})
		}
	}
}

func TestCRUDJSONAtBodyLimitRemainsAccepted(t *testing.T) {
	body := `{"name":"valid"}`
	body += strings.Repeat(" ", 128*1024-len(body))
	repo := &boundedRequestRepo[models.Category, models.CreateCategoryRequest, models.UpdateCategoryRequest]{
		CreateFn: func(_ context.Context, req models.CreateCategoryRequest) (*models.Category, error) {
			return &models.Category{Name: req.Name}, nil
		},
	}
	resource := Resource[models.Category, models.CreateCategoryRequest, models.UpdateCategoryRequest]{Repo: repo}
	response := httptest.NewRecorder()
	resource.Create(response, httptest.NewRequest("POST", "/category", strings.NewReader(body)))
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
}

func TestCRUDRejectsOversizedQueryIDsBeforeRepository(t *testing.T) {
	calls := 0
	repo := &boundedRequestRepo[models.Category, models.CreateCategoryRequest, models.UpdateCategoryRequest]{
		GetFn:    func(context.Context, string) (*models.Category, error) { calls++; return nil, nil },
		DeleteFn: func(context.Context, string) (*models.Category, error) { calls++; return nil, nil },
		ListFn:   func(context.Context, string) ([]models.Category, error) { calls++; return nil, nil },
	}
	resource := Resource[models.Category, models.CreateCategoryRequest, models.UpdateCategoryRequest]{Repo: repo, ListParam: "parentId"}
	for name, handler := range map[string]http.HandlerFunc{"get": resource.Get, "delete": resource.Delete, "list": resource.List} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler(response, httptest.NewRequest("GET", "/?id="+strings.Repeat("x", 129)+"&parentId="+strings.Repeat("x", 129), nil))
			if response.Code != 400 {
				t.Errorf("status=%d; want 400", response.Code)
			}
		})
	}
	if calls != 0 {
		t.Errorf("oversized IDs reached repository %d times", calls)
	}
}

func TestWriteJSONRejectsOversizedSuccessBeforeWriting(t *testing.T) {
	response := httptest.NewRecorder()
	writeJSON(response, http.StatusOK, map[string]string{"private": strings.Repeat("x", 4*1024*1024)})
	if response.Code != 413 {
		t.Fatalf("status=%d; want 413", response.Code)
	}
	if response.Body.Len() > 1024 || strings.Contains(response.Body.String(), "private") {
		t.Error("oversized private result escaped response guard")
	}
}

func TestResultLimitReturnsSafeExplicit413(t *testing.T) {
	repo := &boundedRequestRepo[models.Category, models.CreateCategoryRequest, models.UpdateCategoryRequest]{
		ListFn: func(context.Context, string) ([]models.Category, error) {
			return nil, fmt.Errorf("private-card-content: %w", persistence.ErrResultLimit)
		},
	}
	resource := Resource[models.Category, models.CreateCategoryRequest, models.UpdateCategoryRequest]{Repo: repo}
	response := httptest.NewRecorder()
	resource.List(response, httptest.NewRequest("GET", "/categories", nil))
	if response.Code != 413 || strings.Contains(response.Body.String(), "private-card-content") || !strings.Contains(response.Body.String(), "limit") {
		t.Fatalf("status=%d body=%s; want safe descriptive limit error", response.Code, response.Body)
	}
}

func TestResourceCreateValidatesParentBeforeWriting(t *testing.T) {
	for _, tc := range []struct {
		name            string
		validationError error
		status, calls   int
	}{
		{"parent missing", ErrParentNotFound, 400, 0},
		{"lookup failed", fmt.Errorf("private backend detail"), 500, 0},
		{"parent exists", nil, 201, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls, checks := 0, 0
			repo := &boundedRequestRepo[models.Deck, models.CreateDeckRequest, models.UpdateDeckRequest]{
				CreateFn: func(context.Context, models.CreateDeckRequest) (*models.Deck, error) {
					calls++
					return &models.Deck{}, nil
				},
			}
			resource := Resource[models.Deck, models.CreateDeckRequest, models.UpdateDeckRequest]{
				Repo: repo,
				ValidateCreate: func(_ context.Context, req models.CreateDeckRequest) error {
					checks++
					if req.CategoryId != "parent" {
						t.Errorf("wrong parent: %q", req.CategoryId)
					}
					return tc.validationError
				},
			}
			response := httptest.NewRecorder()
			resource.Create(response, httptest.NewRequest("POST", "/deck", strings.NewReader(`{"name":"deck","categoryID":"parent"}`)))
			if response.Code != tc.status || calls != tc.calls || checks != 1 {
				t.Fatalf("status=%d writes=%d checks=%d; want status=%d writes=%d checks=1", response.Code, calls, checks, tc.status, tc.calls)
			}
			if strings.Contains(response.Body.String(), "private") {
				t.Error("validation error leaked backend detail")
			}
		})
	}
}

func TestRequestFieldBoundsRejectOversizedValues(t *testing.T) {
	for name, request := range map[string]any{
		"category name":        models.CreateCategoryRequest{Name: strings.Repeat("n", 257)},
		"category description": models.UpdateCategoryRequest{Name: "ok", Description: strings.Repeat("d", 8193)},
		"deck parent":          models.CreateDeckRequest{CategoryId: strings.Repeat("p", 129), Name: "ok"},
		"deck name":            models.UpdateDeckRequest{Name: strings.Repeat("n", 257)},
		"tag name":             models.CreateTagRequest{Name: strings.Repeat("n", 257)},
		"tag description":      models.UpdateTagRequest{Name: "ok", Description: strings.Repeat("d", 8193)},
		"card question":        models.CreateCardRequest{DeckId: "deck", Question: strings.Repeat("q", 32769)},
		"card tags":            models.UpdateCardRequest{Question: "ok", TagIds: make([]string, 101)},
		"card tag ID":          models.CreateCardRequest{DeckId: "deck", Question: "ok", TagIds: []string{strings.Repeat("t", 129)}},
		"card timestamp":       models.UpdateCardRequest{Question: "ok", LastAccessedDateTime: strings.Repeat("t", 65)},
		"section title":        models.CreateCardAnswerSectionRequest{CardId: "card", SequenceNumber: 1, Title: strings.Repeat("t", 257)},
		"section answer":       models.UpdateCardAnswerSectionRequest{SequenceNumber: 1, Answer: strings.Repeat("a", 32769)},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validate.Struct(request); err == nil {
				t.Error("oversized field passed validation")
			}
		})
	}
}

// Keep the handler tests independent of storage and the router wiring.
type boundedRequestRepo[T, C, U any] struct {
	ListFn   func(context.Context, string) ([]T, error)
	GetFn    func(context.Context, string) (*T, error)
	CreateFn func(context.Context, C) (*T, error)
	UpdateFn func(context.Context, string, U) (*T, error)
	DeleteFn func(context.Context, string) (*T, error)
}

func (r *boundedRequestRepo[T, C, U]) List(ctx context.Context, id string) ([]T, error) {
	return r.ListFn(ctx, id)
}
func (r *boundedRequestRepo[T, C, U]) Get(ctx context.Context, id string) (*T, error) {
	return r.GetFn(ctx, id)
}
func (r *boundedRequestRepo[T, C, U]) Create(ctx context.Context, req C) (*T, error) {
	return r.CreateFn(ctx, req)
}
func (r *boundedRequestRepo[T, C, U]) Update(ctx context.Context, id string, req U) (*T, error) {
	return r.UpdateFn(ctx, id, req)
}
func (r *boundedRequestRepo[T, C, U]) Delete(ctx context.Context, id string) (*T, error) {
	return r.DeleteFn(ctx, id)
}

func TestResourcePreparesResponseCopiesWithoutChangingStoredEntities(t *testing.T) {
	for _, operation := range []string{"list", "get", "create", "update"} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/fail=%v", operation, fail), func(t *testing.T) {
				stored := models.Category{Id: "category", Name: "stored-value"}
				storedList := []models.Category{stored}
				repo := &boundedRequestRepo[models.Category, models.CreateCategoryRequest, models.UpdateCategoryRequest]{
					ListFn:   func(context.Context, string) ([]models.Category, error) { return storedList, nil },
					GetFn:    func(context.Context, string) (*models.Category, error) { return &stored, nil },
					CreateFn: func(context.Context, models.CreateCategoryRequest) (*models.Category, error) { return &stored, nil },
					UpdateFn: func(context.Context, string, models.UpdateCategoryRequest) (*models.Category, error) {
						return &stored, nil
					},
				}
				preparations := 0
				resource := Resource[models.Category, models.CreateCategoryRequest, models.UpdateCategoryRequest]{
					Repo: repo, Prepare: func(_ context.Context, item *models.Category) error {
						preparations++
						item.Name = "prepared-value"
						if fail {
							return fmt.Errorf("private-signing-failure")
						}
						return nil
					},
				}
				handler := map[string]http.HandlerFunc{"list": resource.List, "get": resource.Get, "create": resource.Create, "update": resource.Update}[operation]
				response := httptest.NewRecorder()
				handler(response, httptest.NewRequest("POST", "/category?id=category", strings.NewReader(`{"name":"valid"}`)))
				status := 200
				if operation == "create" {
					status = 201
				}
				if fail {
					status = 500
				}
				if response.Code != status || preparations != 1 {
					t.Fatalf("status=%d preparations=%d; want %d and 1", response.Code, preparations, status)
				}
				if stored.Name != "stored-value" || storedList[0].Name != "stored-value" {
					t.Errorf("response preparation changed persistent values: %+v %+v", stored, storedList)
				}
				if !fail && !strings.Contains(response.Body.String(), "prepared-value") {
					t.Errorf("prepared field missing from response: %s", response.Body)
				}
				if fail && strings.Contains(response.Body.String(), "private-signing-failure") {
					t.Error("response leaked signing error")
				}
			})
		}
	}
}
