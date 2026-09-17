# FSRS study scheduling

Approved scope: replace Leitner reviews with FSRS and four labelled recall choices, server-owned scheduling, persisted review history, short relearning, interval previews, and migration preserving existing due dates. The user approved implementation in chat on 2026-09-17.

## Behavior

- Use the official Go FSRS library, pinned to a released version, default weights and 0.9 desired retention. Disable fuzz so previews are stable. Configure a single ten-minute learning/relearning step. Again is failure; Hard/Good/Easy are successful recall with decreasing effort.
- Persist scheduling state and a separate immutable review record atomically. The browser sends a rating, idempotency ID and expected revision; the server supplies time and computes the result. Concurrent or stale ratings return 409. Retrying an identical request returns its original review without applying it again.
- Legacy cards retain their last-review-plus-Leitner-interval due date until their first real FSRS review. No fabricated historical events. Missing/invalid old dates are due immediately. On the first review, initialize FSRS conservatively from the available legacy state, clearly separating estimated state from recorded reviews.
- Due cards use persisted schedule.dueAt. New/legacy cards retain a compatibility fallback. A study session keeps short learning/relearning cards pending until due, shows a countdown when only pending cards remain, and supports finishing now without losing their saved schedule. All-card practice also uses actual elapsed time.
- Show four buttons after revealing the answer: Again (Forgot), Hard (Recalled with effort), Good (Recalled), Easy (Recalled easily), each with a server-generated interval. A failed save leaves the current card visible and is safely retryable. Retain existing image zoom/flip behavior.
- Deployment and production data writes are outside this task. No mass rescheduling or destructive migration.

## Shared Go model/API contract

`models.Card` gains `Schedule *models.CardSchedule` (JSON/DynamoDB `schedule`, omitted when nil) and `ReviewRevision uint64` (JSON `reviewRevision`, DynamoDB `review_revision`). Existing fields remain for compatibility.

`models.CardSchedule` fields (JSON camelCase / DynamoDB snake_case): Version int; Algorithm string; DueAt string; Stability float64; Difficulty float64; ScheduledDays uint64; Reps uint64; Lapses uint64; State string (`new`, `learning`, `review`, `relearning`); LastReviewAt string; RemainingSteps int. Dates are UTC RFC3339Nano. Version=1 and Algorithm=`fsrs-6` for new state; legacy snapshots use Algorithm=`leitner`.

`models.ReviewRating` is a string (`again`, `hard`, `good`, `easy`).

`models.CardReview` fields: Id string (JSON id, DynamoDB id), EntityType string (`card_review`), CardId string (JSON cardId / DynamoDB card_id), RequestId string, Rating ReviewRating, ReviewedAt string, PreviousSchedule CardSchedule, Schedule CardSchedule, Revision uint64. All other fields have camelCase JSON and snake_case DynamoDB tags.

Internal cleanup metadata (omitted from JSON): Card.LastReviewId and CardReview.PreviousReviewId link immutable events. SaveReview updates the head atomically and rejects a card with review_deleting set. DeleteReviews marks deletion, walks strongly consistent history reads and transactionally deletes each head while advancing the pointer; a failed cleanup resumes on retry. This avoids GSI lag causing orphaned review records.

`models.ReviewOption`: Rating ReviewRating, DueAt string, IntervalSeconds int64, State string.
`models.ReviewOptions`: CardId string, Revision uint64, GeneratedAt string, Options []ReviewOption.
`models.ReviewCardRequest`: ReviewId string, Rating ReviewRating, ExpectedRevision *uint64 (required, zero valid).
`models.ReviewCardResponse`: Card Card, Review CardReview.

- GET `/card-review-options?cardId=…` -> ReviewOptions. Read only; never advances card state.
- POST `/card-review?cardId=…` + ReviewCardRequest -> ReviewCardResponse (200, including retry).
- GET `/card-reviews?cardId=…` -> CardReview[], chronological.
- Invalid fields 400, malformed JSON 422, missing card 404, conflicting state/request 409. Server failures 500.

## Components and interfaces

`internal/scheduling`: `LegacySchedule(card models.Card, now time.Time) models.CardSchedule`; `Preview(card models.Card, now time.Time) (models.ReviewOptions, error)`; `Apply(card models.Card, rating models.ReviewRating, now time.Time) (models.CardSchedule, error)`. Pure functions, no I/O. Preview and Apply share identical parameters and computation.

`persistence.ReviewStore` interface: `GetCard(ctx context.Context, id string) (*models.Card,error)` (consistent read); `GetReview(ctx context.Context, cardID, requestID string) (*models.CardReview,error)` (consistent read); `SaveReview(ctx context.Context, cardID string, expectedRevision uint64, review models.CardReview) error` (conditional transaction updates only schedule/review_revision/last_accessed_date_time/previously_correct/updated_date_time and puts history); `ListReviews(ctx context.Context, cardID string) ([]models.CardReview,error)`; `DeleteReviews(ctx context.Context, cardID string) error`. `persistence.ErrReviewConflict` denotes conditional conflicts. Constructor `NewReviewStore(*Store) ReviewStore`.

`service.Reviews` owns ReviewStore and injectable Now func() time.Time. Coordinates validation, replay, scheduling, revision checks and return values. `httpapi` maps outcomes to responses. Cascade removes review records before deleting cards; existing card_id-index serves review history with entity_type filtering.

General card editing must not overwrite FSRS state. Legacy schedule writes are permitted only before FSRS state exists, to avoid old clients corrupting a migrated card. Review transactions must preserve concurrently edited question/tags.

## Verification

Scheduler tests cover rating distinctions, ten-minute relearning, long intervals beyond 16 days, elapsed-time sensitivity, preview/apply agreement, and migration without writes/history fabrication. Persistence tests cover transaction contents/conditions, unrelated attribute preservation, duplicate IDs, pagination and errors. Service/API tests cover retries, conflicts, validation, history, missing entities, migration and edit preservation. UI tests cover four previews, posted request, failures/retries, clock-driven relearning, finishing early, resumed pending cards, and existing flip/zoom. Run both complete test suites, go vet and the frontend production build.
