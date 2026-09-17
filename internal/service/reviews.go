package service

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"flashcard_lambda/internal/models"
	"flashcard_lambda/internal/persistence"
	"flashcard_lambda/internal/scheduling"
)

var (
	ErrCardNotFound  = errors.New("card not found")
	ErrInvalidReview = errors.New("invalid review request")
)

// Reviews coordinates server-timed scheduling and atomic state/history writes.
// A review ID belongs to one rating and one revision, including after retries.
type Reviews struct {
	Store persistence.ReviewStore
	Now   func() time.Time
}

func (s *Reviews) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Reviews) card(ctx context.Context, id string) (*models.Card, error) {
	if strings.TrimSpace(id) == "" {
		return nil, ErrInvalidReview
	}
	card, err := s.Store.GetCard(ctx, id)
	if err != nil {
		return nil, err
	}
	if card == nil {
		return nil, ErrCardNotFound
	}
	return card, nil
}

func (s *Reviews) Preview(ctx context.Context, cardID string) (*models.ReviewOptions, error) {
	card, err := s.card(ctx, cardID)
	if err != nil {
		return nil, err
	}
	options, err := scheduling.Preview(*card, s.now())
	if err != nil {
		return nil, err
	}
	return &options, nil
}

func validateReview(req models.ReviewCardRequest) error {
	if strings.TrimSpace(req.ReviewId) == "" || len(req.ReviewId) > 128 || req.ExpectedRevision == nil || *req.ExpectedRevision == math.MaxUint64 {
		return ErrInvalidReview
	}
	switch req.Rating {
	case "again", "hard", "good", "easy":
		return nil
	default:
		return ErrInvalidReview
	}
}

func (s *Reviews) replay(ctx context.Context, cardID string, req models.ReviewCardRequest, review *models.CardReview) (*models.ReviewCardResponse, error) {
	if review.Rating != req.Rating || review.Revision != *req.ExpectedRevision+1 {
		return nil, persistence.ErrReviewConflict
	}
	card, err := s.card(ctx, cardID)
	if err != nil {
		return nil, err
	}
	// Return current card state, even if another valid review followed this
	// event, so a delayed retry cannot roll a client's cached card backwards.
	return &models.ReviewCardResponse{Card: *card, Review: *review}, nil
}

func (s *Reviews) Review(ctx context.Context, cardID string, req models.ReviewCardRequest) (*models.ReviewCardResponse, error) {
	if err := validateReview(req); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cardID) == "" {
		return nil, ErrInvalidReview
	}
	previous, err := s.Store.GetReview(ctx, cardID, req.ReviewId)
	if err != nil {
		return nil, err
	}
	if previous != nil {
		return s.replay(ctx, cardID, req, previous)
	}

	card, err := s.card(ctx, cardID)
	if err != nil {
		return nil, err
	}
	if card.ReviewRevision != *req.ExpectedRevision {
		return nil, persistence.ErrReviewConflict
	}
	now := s.now()
	next, err := scheduling.Apply(*card, req.Rating, now)
	if err != nil {
		return nil, err
	}
	before := scheduling.LegacySchedule(*card, now)
	if card.Schedule != nil {
		before = *card.Schedule
	}
	review := models.CardReview{
		Id:               persistence.ReviewID(cardID, req.ReviewId),
		EntityType:       "card_review",
		CardId:           cardID,
		RequestId:        req.ReviewId,
		Rating:           req.Rating,
		ReviewedAt:       now.Format(time.RFC3339Nano),
		PreviousSchedule: before,
		Schedule:         next,
		Revision:         *req.ExpectedRevision + 1,
		PreviousReviewId: card.LastReviewId,
	}
	if err := s.Store.SaveReview(ctx, cardID, *req.ExpectedRevision, review); err != nil {
		// A competing retry can win the transaction after our first lookup.
		// An uncertain network response can also hide a successful commit.
		if saved, readErr := s.Store.GetReview(ctx, cardID, req.ReviewId); readErr == nil && saved != nil {
			return s.replay(ctx, cardID, req, saved)
		}
		return nil, err
	}
	// Read back to include concurrent content edits without ever writing a
	// stale question or tag list as part of a review.
	return s.replay(ctx, cardID, req, &review)
}

func (s *Reviews) History(ctx context.Context, cardID string) ([]models.CardReview, error) {
	if _, err := s.card(ctx, cardID); err != nil {
		return nil, err
	}
	reviews, err := s.Store.ListReviews(ctx, cardID)
	if err != nil {
		return nil, err
	}
	if reviews == nil {
		reviews = []models.CardReview{}
	}
	sort.Slice(reviews, func(i, j int) bool { return reviews[i].Revision < reviews[j].Revision })
	return reviews, nil
}
