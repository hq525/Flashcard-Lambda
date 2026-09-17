package service

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"flashcard_lambda/internal/models"
	"flashcard_lambda/internal/persistence"
)

var reviewTime = time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

type memoryReviewStore struct {
	card             *models.Card
	reviews          map[string]models.CardReview
	saves            int
	err              error
	afterCommitError error
}

func (s *memoryReviewStore) GetCard(_ context.Context, id string) (*models.Card, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.card == nil || s.card.Id != id {
		return nil, nil
	}
	c := *s.card
	return &c, nil
}
func (s *memoryReviewStore) GetReview(_ context.Context, cardID, requestID string) (*models.CardReview, error) {
	if s.err != nil {
		return nil, s.err
	}
	r, ok := s.reviews[cardID+"/"+requestID]
	if !ok {
		return nil, nil
	}
	return &r, nil
}
func (s *memoryReviewStore) SaveReview(_ context.Context, cardID string, expected uint64, r models.CardReview) error {
	if s.err != nil {
		return s.err
	}
	if s.card == nil || s.card.Id != cardID || s.card.ReviewRevision != expected {
		return persistence.ErrReviewConflict
	}
	if _, exists := s.reviews[cardID+"/"+r.RequestId]; exists {
		return persistence.ErrReviewConflict
	}
	s.saves++
	s.card.Schedule = &r.Schedule
	s.card.ReviewRevision = r.Revision
	s.card.LastReviewId = r.Id
	s.card.LastAccessedDateTime = r.ReviewedAt
	s.card.UpdatedDateTime = r.ReviewedAt
	s.card.PreviouslyCorrect = r.Rating != "again"
	s.reviews[cardID+"/"+r.RequestId] = r
	return s.afterCommitError
}
func (s *memoryReviewStore) ListReviews(_ context.Context, cardID string) ([]models.CardReview, error) {
	if s.err != nil {
		return nil, s.err
	}
	var result []models.CardReview
	for _, r := range s.reviews {
		if r.CardId == cardID {
			result = append(result, r)
		}
	}
	return result, nil
}
func (s *memoryReviewStore) DeleteReviews(_ context.Context, cardID string) error {
	for id, r := range s.reviews {
		if r.CardId == cardID {
			delete(s.reviews, id)
		}
	}
	return s.err
}

func reviewFixture() (*Reviews, *memoryReviewStore) {
	s := &memoryReviewStore{card: &models.Card{Id: "card-1", EntityType: models.EntityTypeCard, DeckId: "deck-1", Question: "What is recall?", TagIds: []string{"tag-1"}, LeitnerBox: 2, LastAccessedDateTime: "2026-09-16T12:00:00Z"}, reviews: map[string]models.CardReview{}}
	return &Reviews{Store: s, Now: func() time.Time { return reviewTime }}, s
}

func reviewRequest(rating models.ReviewRating, revision uint64) models.ReviewCardRequest {
	return models.ReviewCardRequest{ReviewId: "request-1", Rating: rating, ExpectedRevision: &revision}
}

func TestReviewPreviewDoesNotMigrateOrWriteLegacyCard(t *testing.T) {
	svc, store := reviewFixture()
	before := *store.card
	preview, err := svc.Preview(context.Background(), "card-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Options) != 4 || preview.Revision != 0 || preview.GeneratedAt != "2026-09-17T12:00:00Z" {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	if !reflect.DeepEqual(before, *store.card) || store.saves != 0 || len(store.reviews) != 0 {
		t.Fatal("preview changed stored state")
	}
}

func TestReviewPersistsHistoryAndPreservesCardContent(t *testing.T) {
	svc, store := reviewFixture()
	result, err := svc.Review(context.Background(), "card-1", reviewRequest("good", 0))
	if err != nil {
		t.Fatal(err)
	}
	if result.Card.Question != "What is recall?" || !reflect.DeepEqual(result.Card.TagIds, []string{"tag-1"}) {
		t.Fatalf("content overwritten: %+v", result.Card)
	}
	if result.Card.ReviewRevision != 1 || result.Card.Schedule == nil || result.Card.Schedule.Algorithm != "fsrs-6" || !result.Card.PreviouslyCorrect {
		t.Fatalf("schedule not saved: %+v", result.Card)
	}
	if result.Review.PreviousSchedule.DueAt != "2026-09-18T12:00:00Z" || result.Review.PreviousSchedule.Algorithm != "leitner" {
		t.Fatalf("legacy schedule lost: %+v", result.Review)
	}
	if result.Review.ReviewedAt != "2026-09-17T12:00:00Z" || result.Review.Rating != "good" || len(store.reviews) != 1 {
		t.Fatalf("incorrect history: %+v", result.Review)
	}
}

func TestReviewRetryReturnsSameEventWithoutAdvancingAgain(t *testing.T) {
	svc, store := reviewFixture()
	req := reviewRequest("again", 0)
	first, err := svc.Review(context.Background(), "card-1", req)
	if err != nil {
		t.Fatal(err)
	}
	svc.Now = func() time.Time { return reviewTime.Add(time.Hour) }
	retry, err := svc.Review(context.Background(), "card-1", req)
	if err != nil {
		t.Fatal(err)
	}
	if store.saves != 1 || !reflect.DeepEqual(first.Review, retry.Review) || retry.Card.ReviewRevision != 1 {
		t.Fatalf("retry applied twice: %+v", retry)
	}
}

func TestReviewRetryAfterUncertainCommitDoesNotDuplicate(t *testing.T) {
	svc, store := reviewFixture()
	store.afterCommitError = errors.New("response lost after transaction committed")
	req := reviewRequest("good", 0)
	_, _ = svc.Review(context.Background(), "card-1", req)
	store.afterCommitError = nil
	result, err := svc.Review(context.Background(), "card-1", req)
	if err != nil {
		t.Fatal(err)
	}
	if store.saves != 1 || result.Card.ReviewRevision != 1 || len(store.reviews) != 1 {
		t.Fatal("uncertain commit replay duplicated the review")
	}
}

func TestReviewRejectsChangedRetryAndStaleTab(t *testing.T) {
	svc, store := reviewFixture()
	if _, err := svc.Review(context.Background(), "card-1", reviewRequest("good", 0)); err != nil {
		t.Fatal(err)
	}
	changedRating := reviewRequest("easy", 0)
	changedRevision := reviewRequest("good", 1)
	staleTab := reviewRequest("hard", 0)
	staleTab.ReviewId = "other-request"
	for _, req := range []models.ReviewCardRequest{changedRating, changedRevision, staleTab} {
		if _, err := svc.Review(context.Background(), "card-1", req); !errors.Is(err, persistence.ErrReviewConflict) {
			t.Fatalf("request %+v: got %v, want conflict", req, err)
		}
	}
	if store.saves != 1 {
		t.Fatal("conflicting requests changed schedule")
	}
}

func TestReviewRejectsMissingRevisionOrInvalidRating(t *testing.T) {
	svc, store := reviewFixture()
	missingRevision := reviewRequest("good", 0)
	missingRevision.ExpectedRevision = nil
	missingID := reviewRequest("good", 0)
	missingID.ReviewId = ""
	for _, req := range []models.ReviewCardRequest{missingRevision, missingID, reviewRequest("almost", 0)} {
		if _, err := svc.Review(context.Background(), "card-1", req); !errors.Is(err, ErrInvalidReview) {
			t.Fatalf("got %v for %+v", err, req)
		}
	}
	if store.saves != 0 {
		t.Fatal("invalid request changed schedule")
	}
}

func TestReviewNotFoundAndStorageFailure(t *testing.T) {
	svc, store := reviewFixture()
	if _, err := svc.Preview(context.Background(), "missing"); !errors.Is(err, ErrCardNotFound) {
		t.Fatalf("preview: %v", err)
	}
	if _, err := svc.Review(context.Background(), "missing", reviewRequest("good", 0)); !errors.Is(err, ErrCardNotFound) {
		t.Fatalf("review: %v", err)
	}
	if _, err := svc.History(context.Background(), "missing"); !errors.Is(err, ErrCardNotFound) {
		t.Fatalf("history: %v", err)
	}
	store.err = errors.New("storage unavailable")
	if _, err := svc.Review(context.Background(), "card-1", reviewRequest("good", 0)); !errors.Is(err, store.err) {
		t.Fatalf("storage error swallowed: %v", err)
	}
}

func TestReviewHistoryOnlyContainsRealReviews(t *testing.T) {
	svc, _ := reviewFixture()
	before, err := svc.History(context.Background(), "card-1")
	if err != nil || len(before) != 0 {
		t.Fatalf("invented legacy history: %+v %v", before, err)
	}
	if _, err = svc.Review(context.Background(), "card-1", reviewRequest("good", 0)); err != nil {
		t.Fatal(err)
	}
	after, err := svc.History(context.Background(), "card-1")
	if err != nil || len(after) != 1 || after[0].Rating != "good" {
		t.Fatalf("missing real history: %+v %v", after, err)
	}
}

func TestReviewHistoryLinksAllowConsistentDeletionAndOldRetriesKeepLatestState(t *testing.T) {
	svc, store := reviewFixture()
	firstReq := reviewRequest("again", 0)
	first, err := svc.Review(context.Background(), "card-1", firstReq)
	if err != nil {
		t.Fatal(err)
	}
	svc.Now = func() time.Time { return reviewTime.Add(10 * time.Minute) }
	secondReq := reviewRequest("good", 1)
	secondReq.ReviewId = "request-2"
	second, err := svc.Review(context.Background(), "card-1", secondReq)
	if err != nil {
		t.Fatal(err)
	}
	if second.Review.PreviousReviewId != first.Review.Id || store.card.LastReviewId != second.Review.Id {
		t.Fatal("review history chain broken")
	}
	retry, err := svc.Review(context.Background(), "card-1", firstReq)
	if err != nil {
		t.Fatal(err)
	}
	if retry.Review.Id != first.Review.Id || retry.Card.ReviewRevision != 2 || retry.Card.Schedule.State != "review" || store.saves != 2 {
		t.Fatalf("old retry rolled back card state: %+v", retry)
	}
}
