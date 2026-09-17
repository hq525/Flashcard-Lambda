package models

// CardSchedule is the current scheduling state; it is not a review-history event.
type CardSchedule struct {
	Version        int     `json:"version" dynamodbav:"version"`
	Algorithm      string  `json:"algorithm" dynamodbav:"algorithm"`
	DueAt          string  `json:"dueAt" dynamodbav:"due_at"`
	Stability      float64 `json:"stability" dynamodbav:"stability"`
	Difficulty     float64 `json:"difficulty" dynamodbav:"difficulty"`
	ScheduledDays  uint64  `json:"scheduledDays" dynamodbav:"scheduled_days"`
	Reps           uint64  `json:"reps" dynamodbav:"reps"`
	Lapses         uint64  `json:"lapses" dynamodbav:"lapses"`
	State          string  `json:"state" dynamodbav:"state"`
	LastReviewAt   string  `json:"lastReviewAt" dynamodbav:"last_review_at"`
	RemainingSteps int     `json:"remainingSteps" dynamodbav:"remaining_steps"`
}

type ReviewRating string

const (
	RatingAgain ReviewRating = "again"
	RatingHard  ReviewRating = "hard"
	RatingGood  ReviewRating = "good"
	RatingEasy  ReviewRating = "easy"
)

// CardReview is an immutable record of one observed, server-timed review.
type CardReview struct {
	Id               string       `json:"id" dynamodbav:"id"`
	EntityType       string       `json:"entityType" dynamodbav:"entity_type"`
	CardId           string       `json:"cardId" dynamodbav:"card_id"`
	RequestId        string       `json:"requestId" dynamodbav:"request_id"`
	Rating           ReviewRating `json:"rating" dynamodbav:"rating"`
	ReviewedAt       string       `json:"reviewedAt" dynamodbav:"reviewed_at"`
	PreviousSchedule CardSchedule `json:"previousSchedule" dynamodbav:"previous_schedule"`
	Schedule         CardSchedule `json:"schedule" dynamodbav:"schedule"`
	Revision         uint64       `json:"revision" dynamodbav:"revision"`
	PreviousReviewId string       `json:"-" dynamodbav:"previous_review_id,omitempty"`
}

type ReviewOption struct {
	Rating          ReviewRating `json:"rating"`
	DueAt           string       `json:"dueAt"`
	IntervalSeconds int64        `json:"intervalSeconds"`
	State           string       `json:"state"`
}

type ReviewOptions struct {
	CardId      string         `json:"cardId"`
	Revision    uint64         `json:"revision"`
	GeneratedAt string         `json:"generatedAt"`
	Options     []ReviewOption `json:"options"`
}

type ReviewCardRequest struct {
	ReviewId         string       `json:"reviewId"`
	Rating           ReviewRating `json:"rating"`
	ExpectedRevision *uint64      `json:"expectedRevision"`
}

type ReviewCardResponse struct {
	Card   Card       `json:"card"`
	Review CardReview `json:"review"`
}
