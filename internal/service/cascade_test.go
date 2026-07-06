package service

import (
	"context"
	"errors"
	"testing"

	"flashcard_lambda/internal/models"
	"flashcard_lambda/internal/testutil"
)

// buildFixture wires a Cascade over fakes representing:
//
//	category cat-1
//	└── deck deck-1
//	    └── card card-1
//	        ├── question image qi-1 (with S3 object)
//	        └── section sec-1
//	            └── section image si-1 (with S3 object)
func buildFixture() (*Cascade, *testutil.FakeImageStore, map[string]*[]string) {
	images := &testutil.FakeImageStore{}

	categories := &testutil.FakeRepo[models.Category, models.CreateCategoryRequest, models.UpdateCategoryRequest]{
		DeleteFn: func(ctx context.Context, id string) (*models.Category, error) {
			return &models.Category{Id: id}, nil
		},
	}
	decks := &testutil.FakeRepo[models.Deck, models.CreateDeckRequest, models.UpdateDeckRequest]{
		ListFn: func(ctx context.Context, parentID string) ([]models.Deck, error) {
			if parentID == "cat-1" {
				return []models.Deck{{Id: "deck-1", CategoryId: "cat-1"}}, nil
			}
			return nil, nil
		},
		DeleteFn: func(ctx context.Context, id string) (*models.Deck, error) {
			return &models.Deck{Id: id}, nil
		},
	}
	cards := &testutil.FakeRepo[models.Card, models.CreateCardRequest, models.UpdateCardRequest]{
		ListFn: func(ctx context.Context, parentID string) ([]models.Card, error) {
			if parentID == "deck-1" {
				return []models.Card{{Id: "card-1", DeckId: "deck-1"}}, nil
			}
			return nil, nil
		},
		DeleteFn: func(ctx context.Context, id string) (*models.Card, error) {
			return &models.Card{Id: id}, nil
		},
	}
	sections := &testutil.FakeRepo[models.CardAnswerSection, models.CreateCardAnswerSectionRequest, models.UpdateCardAnswerSectionRequest]{
		ListFn: func(ctx context.Context, parentID string) ([]models.CardAnswerSection, error) {
			if parentID == "card-1" {
				return []models.CardAnswerSection{{Id: "sec-1", CardId: "card-1"}}, nil
			}
			return nil, nil
		},
		DeleteFn: func(ctx context.Context, id string) (*models.CardAnswerSection, error) {
			return &models.CardAnswerSection{Id: id}, nil
		},
	}
	questionImages := &testutil.FakeRepo[models.CardQuestionImage, models.CreateCardQuestionImageRequest, models.UpdateCardQuestionImageRequest]{
		ListFn: func(ctx context.Context, parentID string) ([]models.CardQuestionImage, error) {
			if parentID == "card-1" {
				return []models.CardQuestionImage{{Id: "qi-1", CardId: "card-1", ImageURL: "https://b.s3.amazonaws.com/question-images/qi-1.png"}}, nil
			}
			return nil, nil
		},
		DeleteFn: func(ctx context.Context, id string) (*models.CardQuestionImage, error) {
			return &models.CardQuestionImage{Id: id, ImageURL: "https://b.s3.amazonaws.com/question-images/qi-1.png"}, nil
		},
	}
	sectionImages := &testutil.FakeRepo[models.CardAnswerSectionImage, models.CreateCardAnswerSectionImageRequest, models.UpdateCardAnswerSectionImageRequest]{
		ListFn: func(ctx context.Context, parentID string) ([]models.CardAnswerSectionImage, error) {
			if parentID == "sec-1" {
				return []models.CardAnswerSectionImage{{Id: "si-1", CardAnswerSectionId: "sec-1", ImageURL: "https://b.s3.amazonaws.com/answer-images/si-1.png"}}, nil
			}
			return nil, nil
		},
		DeleteFn: func(ctx context.Context, id string) (*models.CardAnswerSectionImage, error) {
			return &models.CardAnswerSectionImage{Id: id, ImageURL: "https://b.s3.amazonaws.com/answer-images/si-1.png"}, nil
		},
	}

	cascade := &Cascade{
		Categories:     categories,
		Decks:          decks,
		Cards:          cards,
		Sections:       sections,
		QuestionImages: questionImages,
		SectionImages:  sectionImages,
		Images:         images,
	}
	deleted := map[string]*[]string{
		"categories":     &categories.Deleted,
		"decks":          &decks.Deleted,
		"cards":          &cards.Deleted,
		"sections":       &sections.Deleted,
		"questionImages": &questionImages.Deleted,
		"sectionImages":  &sectionImages.Deleted,
	}
	return cascade, images, deleted
}

func TestDeleteCategoryCascades(t *testing.T) {
	cascade, images, deleted := buildFixture()

	category, err := cascade.DeleteCategory(context.Background(), "cat-1")
	if err != nil {
		t.Fatalf("DeleteCategory: %v", err)
	}
	if category == nil || category.Id != "cat-1" {
		t.Fatalf("DeleteCategory returned %+v", category)
	}

	want := map[string][]string{
		"categories":     {"cat-1"},
		"decks":          {"deck-1"},
		"cards":          {"card-1"},
		"sections":       {"sec-1"},
		"questionImages": {"qi-1"},
		"sectionImages":  {"si-1"},
	}
	for name, ids := range want {
		got := *deleted[name]
		if len(got) != len(ids) || (len(got) > 0 && got[0] != ids[0]) {
			t.Errorf("%s deleted = %v, want %v", name, got, ids)
		}
	}

	if len(images.DeletedURL) != 2 {
		t.Errorf("S3 deletes = %v, want the question image and section image objects", images.DeletedURL)
	}
}

func TestDeleteCardFailureLeavesCardIntact(t *testing.T) {
	cascade, _, deleted := buildFixture()
	boom := errors.New("dynamo unavailable")
	cascade.Sections.(*testutil.FakeRepo[models.CardAnswerSection, models.CreateCardAnswerSectionRequest, models.UpdateCardAnswerSectionRequest]).ListFn =
		func(ctx context.Context, parentID string) ([]models.CardAnswerSection, error) {
			return nil, boom
		}

	if _, err := cascade.DeleteCard(context.Background(), "card-1"); !errors.Is(err, boom) {
		t.Fatalf("DeleteCard error = %v, want %v", err, boom)
	}
	if got := *deleted["cards"]; len(got) != 0 {
		t.Errorf("card record deleted despite child failure: %v", got)
	}
}

func TestDeleteQuestionImageToleratesS3Failure(t *testing.T) {
	cascade, images, _ := buildFixture()
	images.DeleteErr = errors.New("s3 unavailable")

	image, err := cascade.DeleteQuestionImage(context.Background(), "qi-1")
	if err != nil {
		t.Fatalf("S3 failure should not fail the request, got %v", err)
	}
	if image == nil {
		t.Fatal("expected deleted image record")
	}
	if len(images.DeletedURL) != 1 {
		t.Errorf("expected one attempted S3 delete, got %v", images.DeletedURL)
	}
}

func TestDeleteQuestionImageNotFoundSkipsS3(t *testing.T) {
	cascade, images, _ := buildFixture()
	cascade.QuestionImages.(*testutil.FakeRepo[models.CardQuestionImage, models.CreateCardQuestionImageRequest, models.UpdateCardQuestionImageRequest]).DeleteFn =
		func(ctx context.Context, id string) (*models.CardQuestionImage, error) {
			return nil, nil
		}

	image, err := cascade.DeleteQuestionImage(context.Background(), "missing")
	if err != nil || image != nil {
		t.Fatalf("expected (nil, nil) for missing image, got (%v, %v)", image, err)
	}
	if len(images.DeletedURL) != 0 {
		t.Errorf("no S3 delete expected for missing record, got %v", images.DeletedURL)
	}
}
