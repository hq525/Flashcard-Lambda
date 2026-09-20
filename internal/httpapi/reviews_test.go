package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flashcard_lambda/internal/models"
	"flashcard_lambda/internal/persistence"
	"flashcard_lambda/internal/service"
)

type reviewAPIStub struct {
	err    error
	cardID string
	req    models.ReviewCardRequest
	calls  int
}

func (s *reviewAPIStub) Preview(_ context.Context, id string) (*models.ReviewOptions, error) {
	s.calls++
	s.cardID = id
	return &models.ReviewOptions{CardId: id, Revision: 3, GeneratedAt: "2026-09-17T12:00:00Z", Options: []models.ReviewOption{{Rating: "again", DueAt: "2026-09-17T12:10:00Z", IntervalSeconds: 600, State: "relearning"}}}, s.err
}
func (s *reviewAPIStub) Review(_ context.Context, id string, req models.ReviewCardRequest) (*models.ReviewCardResponse, error) {
	s.calls++
	s.cardID = id
	s.req = req
	return &models.ReviewCardResponse{Card: models.Card{Id: id, ReviewRevision: 4}, Review: models.CardReview{Rating: req.Rating, Revision: 4}}, s.err
}
func (s *reviewAPIStub) History(_ context.Context, id string) ([]models.CardReview, error) {
	s.calls++
	s.cardID = id
	return nil, s.err
}
func reviewAPIRequest(stub *reviewAPIStub, method, path, body string) *httptest.ResponseRecorder {
	router := NewRouter(Deps{Reviews: stub, Cascade: &service.Cascade{}, Authenticate: allowTestRequests, AllowedOrigin: "http://localhost:5173"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Origin", "http://localhost:5173")
	router.ServeHTTP(rec, req)
	return rec
}

func TestReviewAPIExposesServerPreviewAndDecodedRating(t *testing.T) {
	stub := &reviewAPIStub{}
	preview := reviewAPIRequest(stub, "GET", "/card-review-options?cardId=card-1", "")
	if preview.Code != 200 {
		t.Fatalf("preview: %d %s", preview.Code, preview.Body)
	}
	var options models.ReviewOptions
	if err := json.Unmarshal(preview.Body.Bytes(), &options); err != nil {
		t.Fatal(err)
	}
	if options.Revision != 3 || options.Options[0].IntervalSeconds != 600 || stub.cardID != "card-1" {
		t.Fatalf("wrong preview: %+v", options)
	}
	response := reviewAPIRequest(stub, "POST", "/card-review?cardId=card-1", `{"reviewId":"request-1","rating":"good","expectedRevision":3}`)
	if response.Code != 200 {
		t.Fatalf("review: %d %s", response.Code, response.Body)
	}
	if stub.req.ReviewId != "request-1" || stub.req.Rating != "good" || stub.req.ExpectedRevision == nil || *stub.req.ExpectedRevision != 3 {
		t.Fatalf("wrong decoded request: %+v", stub.req)
	}
}

func TestReviewAPIRejectsInvalidPayloadBeforeCallingService(t *testing.T) {
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{`, 422},
		{`null`, 400},
		{`{"rating":"good","expectedRevision":0}`, 400},
		{`{"reviewId":"r","rating":"good"}`, 400},
		{`{"reviewId":"r","rating":"almost","expectedRevision":0}`, 400},
		{`{"reviewId":"r","rating":"good","expectedRevision":-1}`, 422},
		{`{"reviewId":"r","rating":"good","expectedRevision":0,"dueAt":"2099-01-01"}`, 422},
		{`{"reviewId":"r","rating":"good","expectedRevision":0} {}`, 422},
	} {
		stub := &reviewAPIStub{}
		result := reviewAPIRequest(stub, "POST", "/card-review?cardId=card-1", tc.body)
		if result.Code != tc.status || stub.calls != 0 {
			t.Errorf("%s: status %d, calls %d; want %d, zero calls", tc.body, result.Code, stub.calls, tc.status)
		}
	}
}

func TestReviewAPIRequiresCardIDAndAcceptsRevisionZero(t *testing.T) {
	for _, path := range []string{"/card-review-options", "/card-reviews"} {
		if got := reviewAPIRequest(&reviewAPIStub{}, "GET", path, "").Code; got != 400 {
			t.Errorf("%s: %d", path, got)
		}
	}
	if got := reviewAPIRequest(&reviewAPIStub{}, "POST", "/card-review", `{}`).Code; got != 400 {
		t.Errorf("missing cardId: %d", got)
	}
	if got := reviewAPIRequest(&reviewAPIStub{}, "POST", "/card-review?cardId=card-1", `{"reviewId":"r","rating":"again","expectedRevision":0}`).Code; got != 200 {
		t.Errorf("zero revision: %d", got)
	}
}

func TestReviewAPIMapsErrorsAndKeepsCORS(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{
		{service.ErrCardNotFound, 404},
		{service.ErrInvalidReview, 400},
		{persistence.ErrReviewConflict, 409},
		{errors.New("database failed"), 500},
	} {
		result := reviewAPIRequest(&reviewAPIStub{err: tc.err}, "POST", "/card-review?cardId=card-1", `{"reviewId":"r","rating":"good","expectedRevision":0}`)
		if result.Code != tc.status || result.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
			t.Errorf("%v: %d %s", tc.err, result.Code, result.Body)
		}
	}
}

func TestReviewHistoryEmptyJSONIsArray(t *testing.T) {
	result := reviewAPIRequest(&reviewAPIStub{}, "GET", "/card-reviews?cardId=card-1", "")
	if result.Code != http.StatusOK || strings.TrimSpace(result.Body.String()) != "[]" {
		t.Fatalf("%d %s", result.Code, result.Body)
	}
}
