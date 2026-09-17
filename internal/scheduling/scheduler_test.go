package scheduling

import (
	"flashcard_lambda/internal/models"
	"reflect"
	"testing"
	"time"
)

var testNow = time.Date(2026, time.September, 17, 12, 0, 0, 123456789, time.UTC)

func TestNewCardRatingsAndPreviewMatchApply(t *testing.T) {
	card := models.Card{Id: "new", ReviewRevision: 7}
	preview, err := Preview(card, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if preview.CardId != "new" || preview.Revision != 7 || preview.GeneratedAt != "2026-09-17T12:00:00.123456789Z" {
		t.Fatalf("unexpected preview metadata: %+v", preview)
	}
	if len(preview.Options) != 4 {
		t.Fatalf("got %d options, want four", len(preview.Options))
	}
	wantRatings := []models.ReviewRating{"again", "hard", "good", "easy"}
	var previous int64
	for i, option := range preview.Options {
		if option.Rating != wantRatings[i] {
			t.Errorf("rating %d = %q", i, option.Rating)
		}
		result, err := Apply(card, option.Rating, testNow)
		if err != nil {
			t.Fatal(err)
		}
		if result.DueAt != option.DueAt || result.State != option.State {
			t.Errorf("preview/apply mismatch: %+v, %+v", option, result)
		}
		due, err := time.Parse(time.RFC3339Nano, result.DueAt)
		if err != nil {
			t.Fatal(err)
		}
		if int64(due.Sub(testNow)/time.Second) != option.IntervalSeconds {
			t.Errorf("wrong interval seconds: %+v", option)
		}
		if option.IntervalSeconds <= previous {
			t.Errorf("rating intervals not increasing: %+v", preview.Options)
		}
		previous = option.IntervalSeconds
		if result.Algorithm != "fsrs-6" || result.Version != 1 || result.Reps != 1 || result.LastReviewAt != preview.GeneratedAt {
			t.Errorf("wrong recorded state: %+v", result)
		}
	}
	if preview.Options[0].IntervalSeconds != 600 || preview.Options[0].State != "learning" {
		t.Errorf("Again must enter 10-minute learning: %+v", preview.Options[0])
	}
	if card.Schedule != nil {
		t.Fatal("preview/apply mutated input card")
	}
}

func TestLegacySchedulePreservesDueWithoutInventedReviews(t *testing.T) {
	for _, tc := range []struct {
		box  uint8
		due  string
		days uint64
	}{
		{0, "2026-09-02T12:00:00Z", 1}, {1, "2026-09-02T12:00:00Z", 1},
		{2, "2026-09-03T12:00:00Z", 2}, {3, "2026-09-05T12:00:00Z", 4},
		{4, "2026-09-09T12:00:00Z", 8}, {5, "2026-09-17T12:00:00Z", 16}, {255, "2026-09-17T12:00:00Z", 16},
	} {
		card := models.Card{LeitnerBox: tc.box, LastAccessedDateTime: "2026-09-01T20:00:00+08:00"}
		legacy := LegacySchedule(card, testNow)
		if legacy.Algorithm != "leitner" || legacy.DueAt != tc.due || legacy.ScheduledDays != tc.days || legacy.Reps != 0 || legacy.Lapses != 0 {
			t.Errorf("box %d: %+v", tc.box, legacy)
		}
		before := legacy
		if _, err := Preview(card, testNow); err != nil {
			t.Fatal(err)
		}
		if card.Schedule != nil || !reflect.DeepEqual(before, LegacySchedule(card, testNow)) {
			t.Fatal("preview mutated legacy state")
		}
		migrated, err := Apply(card, "good", testNow)
		if err != nil {
			t.Fatal(err)
		}
		if migrated.Algorithm != "fsrs-6" || migrated.Reps != 1 || migrated.LastReviewAt != "2026-09-17T12:00:00.123456789Z" {
			t.Errorf("migration fabricated reviews: %+v", migrated)
		}
	}
}

func TestMissingOrInvalidLegacyDatesAreNewAndDueImmediately(t *testing.T) {
	for _, value := range []string{"", "garbage", "0001-01-01T00:00:00Z", "2026-09-18T12:00:00Z"} {
		card := models.Card{LastAccessedDateTime: value, LeitnerBox: 5}
		legacy := LegacySchedule(card, testNow)
		if legacy.DueAt != "2026-09-17T12:00:00.123456789Z" || legacy.State != "new" || legacy.LastReviewAt != "" || legacy.Reps != 0 {
			t.Errorf("invalid date %q: %+v", value, legacy)
		}
		got, err := Apply(card, "again", testNow)
		if err != nil {
			t.Fatal(err)
		}
		if got.State != "learning" || got.Lapses != 0 || got.Reps != 1 {
			t.Errorf("invalid legacy treated as existing review: %+v", got)
		}
	}
}

func TestLegacyMigrationRetainsMemoryWithoutHistoricalCounts(t *testing.T) {
	card := models.Card{LeitnerBox: 5, LastAccessedDateTime: "2026-09-17T11:00:00Z"}
	result, err := Apply(card, "good", testNow)
	if err != nil {
		t.Fatal(err)
	}
	// A box-5 card has a 16-day interval. Same-day practice must retain that
	// existing memory without resetting to the initial new-card interval or
	// claiming a full additional 16 days have elapsed.
	if result.ScheduledDays < 16 || result.ScheduledDays >= 32 {
		t.Errorf("legacy memory estimate lost or automatically doubled: %+v", result)
	}
	if result.Reps != 1 || result.Lapses != 0 {
		t.Errorf("legacy migration invented review counts: %+v", result)
	}
}

func establishedCard() models.Card {
	return models.Card{Id: "review", Schedule: &models.CardSchedule{
		Version: 1, Algorithm: "fsrs-6", DueAt: "2026-09-17T12:00:00Z",
		Stability: 16, Difficulty: 5, ScheduledDays: 16, Reps: 8, Lapses: 2,
		State: "review", LastReviewAt: "2026-09-01T12:00:00Z",
	}}
}

func TestForgottenCardRelearnsAfterTenMinutesAndGraduates(t *testing.T) {
	card := establishedCard()
	failed, err := Apply(card, "again", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if failed.State != "relearning" || failed.DueAt != "2026-09-17T12:10:00.123456789Z" || failed.RemainingSteps != 1 || failed.Lapses != 3 || failed.Reps != 9 {
		t.Fatalf("wrong relearning state: %+v", failed)
	}
	if failed.Stability >= card.Schedule.Stability {
		t.Errorf("failure did not lower stability: %+v", failed)
	}
	card.Schedule = &failed
	now := testNow.Add(10 * time.Minute)
	again, err := Apply(card, "again", now)
	if err != nil {
		t.Fatal(err)
	}
	if again.State != "relearning" || again.DueAt != "2026-09-17T12:20:00.123456789Z" || again.Lapses != 3 {
		t.Errorf("repeat failure should stay in relearning: %+v", again)
	}
	good, err := Apply(card, "good", now)
	if err != nil {
		t.Fatal(err)
	}
	if good.State != "review" || good.RemainingSteps != 0 || good.ScheduledDays < 1 || good.Lapses != 3 {
		t.Errorf("successful relearning did not graduate: %+v", good)
	}
}

func TestSuccessfulReviewCanExceedSixteenDays(t *testing.T) {
	preview, err := Preview(establishedCard(), testNow)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Options[2].IntervalSeconds <= 16*86400 {
		t.Fatalf("Good still capped at 16 days: %+v", preview.Options)
	}
	for i := 1; i < len(preview.Options); i++ {
		if preview.Options[i].IntervalSeconds <= preview.Options[i-1].IntervalSeconds {
			t.Errorf("ratings not distinct: %+v", preview.Options)
		}
	}
}

func TestEarlyAndSameDayReviewsUseElapsedTime(t *testing.T) {
	card := establishedCard()
	last, _ := time.Parse(time.RFC3339Nano, card.Schedule.LastReviewAt)
	early, err := Apply(card, "good", last.Add(10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	due, err := Apply(card, "good", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if early.ScheduledDays >= 32 {
		t.Errorf("same-day review automatically doubled interval: %+v", early)
	}
	if early.Stability >= due.Stability || early.ScheduledDays >= due.ScheduledDays {
		t.Errorf("elapsed time ignored: early %+v, due %+v", early, due)
	}
	card.Schedule = &early
	repeated, err := Apply(card, "good", last.Add(20*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if repeated.ScheduledDays >= early.ScheduledDays*2 {
		t.Errorf("repeated same-day review doubled interval: %+v", repeated)
	}
}

func TestPreviewIsDeterministicAndLeavesStoredStateAlone(t *testing.T) {
	card := establishedCard()
	before := *card.Schedule
	one, err := Preview(card, testNow)
	if err != nil {
		t.Fatal(err)
	}
	two, err := Preview(card, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(one, two) {
		t.Fatalf("preview changed without state or time changing: %+v, %+v", one, two)
	}
	if *card.Schedule != before {
		t.Fatal("preview mutated persisted schedule")
	}
	for _, option := range one.Options {
		applied, err := Apply(card, option.Rating, testNow)
		if err != nil {
			t.Fatal(err)
		}
		if applied.DueAt != option.DueAt || applied.State != option.State {
			t.Errorf("preview/apply mismatch: %+v, %+v", option, applied)
		}
	}
}

func TestRejectsInvalidRatingOrStoredSchedule(t *testing.T) {
	for _, rating := range []models.ReviewRating{"", "Good", "forgot", "5"} {
		if _, err := Apply(models.Card{}, rating, testNow); err == nil {
			t.Errorf("accepted rating %q", rating)
		}
	}
	for _, mutate := range []func(*models.CardSchedule){
		func(s *models.CardSchedule) { s.Version = 2 },
		func(s *models.CardSchedule) { s.Algorithm = "unknown" },
		func(s *models.CardSchedule) { s.State = "unknown" },
		func(s *models.CardSchedule) { s.LastReviewAt = "garbage" },
		func(s *models.CardSchedule) { s.LastReviewAt = "" },
		func(s *models.CardSchedule) { s.LastReviewAt = "2026-09-18T00:00:00Z" },
		func(s *models.CardSchedule) { s.DueAt = "garbage" },
		func(s *models.CardSchedule) { s.Stability = -1 },
	} {
		card := establishedCard()
		mutate(card.Schedule)
		if _, err := Preview(card, testNow); err == nil {
			t.Errorf("accepted invalid schedule: %+v", *card.Schedule)
		}
		if _, err := Apply(card, "good", testNow); err == nil {
			t.Errorf("applied invalid schedule: %+v", *card.Schedule)
		}
	}
}
