package scheduling

import (
	"flashcard_lambda/internal/models"
	"fmt"
	"math"
	"time"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v4"
)

// LegacySchedule describes the old due date without fabricating FSRS memory or
// historical review counts. Invalid or future review dates are treated as new.
func LegacySchedule(card models.Card, now time.Time) models.CardSchedule {
	now = now.UTC()
	legacy := models.CardSchedule{Version: 1, Algorithm: "leitner", DueAt: timestamp(now), State: "new"}
	lastReview, err := time.Parse(time.RFC3339Nano, card.LastAccessedDateTime)
	if err != nil || lastReview.IsZero() || lastReview.After(now) {
		return legacy
	}
	box := max(uint8(1), min(card.LeitnerBox, uint8(5)))
	days := uint64(1) << (box - 1)
	legacy.DueAt = timestamp(lastReview.Add(time.Duration(days) * 24 * time.Hour))
	legacy.LastReviewAt = timestamp(lastReview)
	legacy.ScheduledDays = days
	legacy.State = "review"
	return legacy
}

// Preview computes every rating from the same persisted card and reference time.
// It never mutates the card or writes a migrated schedule.
func Preview(card models.Card, now time.Time) (models.ReviewOptions, error) {
	now = now.UTC()
	records, err := outcomes(card, now)
	if err != nil {
		return models.ReviewOptions{}, err
	}
	result := models.ReviewOptions{
		CardId: card.Id, Revision: card.ReviewRevision, GeneratedAt: timestamp(now),
		Options: make([]models.ReviewOption, 0, 4),
	}
	for _, rating := range []models.ReviewRating{models.RatingAgain, models.RatingHard, models.RatingGood, models.RatingEasy} {
		grade, _ := fsrsRating(rating)
		next := records[grade].Card
		result.Options = append(result.Options, models.ReviewOption{
			Rating: rating, DueAt: timestamp(next.Due),
			IntervalSeconds: int64(next.Due.Sub(now) / time.Second), State: stateName(next.State),
		})
	}
	return result, nil
}

// Apply shares Preview's complete scheduling calculation, including the library's
// UTC calendar-day elapsed time and short-term update for same-day reviews.
func Apply(card models.Card, rating models.ReviewRating, now time.Time) (models.CardSchedule, error) {
	grade, err := fsrsRating(rating)
	if err != nil {
		return models.CardSchedule{}, err
	}
	records, err := outcomes(card, now.UTC())
	if err != nil {
		return models.CardSchedule{}, err
	}
	next := records[grade].Card
	return models.CardSchedule{
		Version: 1, Algorithm: "fsrs-6", DueAt: timestamp(next.Due),
		Stability: next.Stability, Difficulty: next.Difficulty,
		ScheduledDays: next.ScheduledDays, Reps: next.Reps, Lapses: next.Lapses,
		State: stateName(next.State), LastReviewAt: timestamp(next.LastReview),
		RemainingSteps: next.RemainingSteps,
	}, nil
}

func outcomes(card models.Card, now time.Time) (fsrs.RecordLog, error) {
	if now.IsZero() {
		return nil, fmt.Errorf("review time is required")
	}
	current, err := fsrsCard(card, now)
	if err != nil {
		return nil, err
	}
	parameters := fsrs.DefaultParam()
	parameters.RequestRetention = 0.9
	parameters.EnableFuzz = false
	parameters.LearningSteps = []float64{10}
	parameters.RelearningSteps = []float64{10}
	return fsrs.NewFSRS(parameters).Repeat(current, now)
}

func fsrsCard(card models.Card, now time.Time) (fsrs.Card, error) {
	if card.Schedule == nil {
		legacy := LegacySchedule(card, now)
		current := fsrs.NewCard(now)
		if legacy.State == "new" {
			return current, nil
		}
		current.Due, _ = time.Parse(time.RFC3339Nano, legacy.DueAt)
		current.LastReview, _ = time.Parse(time.RFC3339Nano, legacy.LastReviewAt)
		current.State = fsrs.Review
		current.ScheduledDays = legacy.ScheduledDays
		// At 90% desired retention, FSRS stability in days is the target interval.
		// The old 1-16 day interval is only an estimate of existing memory. A
		// neutral difficulty of 5 avoids claiming knowledge of past ratings.
		// Reps and Lapses remain zero: only actual FSRS reviews are counted.
		current.Stability = float64(legacy.ScheduledDays)
		current.Difficulty = 5
		return current, nil
	}
	schedule := *card.Schedule
	if schedule.Version != 1 || schedule.Algorithm != "fsrs-6" {
		return fsrs.Card{}, fmt.Errorf("unsupported schedule version or algorithm")
	}
	state, err := fsrsState(schedule.State)
	if err != nil {
		return fsrs.Card{}, err
	}
	due, err := time.Parse(time.RFC3339Nano, schedule.DueAt)
	if err != nil || due.IsZero() {
		return fsrs.Card{}, fmt.Errorf("invalid schedule due date")
	}
	var lastReview time.Time
	if schedule.LastReviewAt != "" {
		lastReview, err = time.Parse(time.RFC3339Nano, schedule.LastReviewAt)
		if err != nil || lastReview.IsZero() {
			return fsrs.Card{}, fmt.Errorf("invalid schedule last review date")
		}
	}
	if state != fsrs.New && lastReview.IsZero() {
		return fsrs.Card{}, fmt.Errorf("reviewed card has no last review date")
	}
	if math.IsNaN(schedule.Stability) || math.IsInf(schedule.Stability, 0) ||
		math.IsNaN(schedule.Difficulty) || math.IsInf(schedule.Difficulty, 0) {
		return fsrs.Card{}, fmt.Errorf("invalid schedule memory state")
	}
	return fsrs.Card{
		Due: due.UTC(), LastReview: lastReview.UTC(), State: state,
		Stability: schedule.Stability, Difficulty: schedule.Difficulty,
		ScheduledDays: schedule.ScheduledDays, Reps: schedule.Reps,
		Lapses: schedule.Lapses, RemainingSteps: schedule.RemainingSteps,
	}, nil
}

func fsrsRating(rating models.ReviewRating) (fsrs.Rating, error) {
	switch rating {
	case models.RatingAgain:
		return fsrs.Again, nil
	case models.RatingHard:
		return fsrs.Hard, nil
	case models.RatingGood:
		return fsrs.Good, nil
	case models.RatingEasy:
		return fsrs.Easy, nil
	default:
		return 0, fmt.Errorf("invalid review rating %q", rating)
	}
}

func fsrsState(state string) (fsrs.State, error) {
	switch state {
	case "new":
		return fsrs.New, nil
	case "learning":
		return fsrs.Learning, nil
	case "review":
		return fsrs.Review, nil
	case "relearning":
		return fsrs.Relearning, nil
	default:
		return 0, fmt.Errorf("invalid schedule state %q", state)
	}
}

func stateName(state fsrs.State) string {
	switch state {
	case fsrs.New:
		return "new"
	case fsrs.Learning:
		return "learning"
	case fsrs.Review:
		return "review"
	case fsrs.Relearning:
		return "relearning"
	default:
		return ""
	}
}

func timestamp(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
