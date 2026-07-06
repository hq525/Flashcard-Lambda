package persistence

import (
	"testing"
	"time"

	"flashcard_lambda/internal/models"
)

// The entity configs are the only per-entity logic left after the generic
// repository collapse; these tests pin down what they produce.

func categoryConfig() EntityConfig[models.Category, models.CreateCategoryRequest, models.UpdateCategoryRequest] {
	repo := NewCategoryRepository(&Store{}).(*DynamoRepository[models.Category, models.CreateCategoryRequest, models.UpdateCategoryRequest])
	return repo.cfg
}

func TestNewCategorySetsIDAndEntityType(t *testing.T) {
	cfg := categoryConfig()
	category := cfg.New(models.CreateCategoryRequest{Name: "Golang", Description: "Go stuff"})

	if category.Id == "" {
		t.Error("New should assign an id")
	}
	if category.EntityType != models.EntityTypeCategory {
		t.Errorf("EntityType = %q, want %q", category.EntityType, models.EntityTypeCategory)
	}
	if category.Name != "Golang" || category.Description != "Go stuff" {
		t.Errorf("unexpected fields: %+v", category)
	}
}

func cardConfig() EntityConfig[models.Card, models.CreateCardRequest, models.UpdateCardRequest] {
	repo := NewCardRepository(&Store{}).(*DynamoRepository[models.Card, models.CreateCardRequest, models.UpdateCardRequest])
	return repo.cfg
}

func TestNewCardSetsTimestamps(t *testing.T) {
	cfg := cardConfig()
	card := cfg.New(models.CreateCardRequest{DeckId: "deck-1", Question: "What is a goroutine?"})

	if card.CreatedDateTime == "" || card.UpdatedDateTime == "" {
		t.Fatalf("timestamps not set: %+v", card)
	}
	if _, err := time.Parse(time.RFC3339, card.CreatedDateTime); err != nil {
		t.Errorf("CreatedDateTime %q is not RFC3339: %v", card.CreatedDateTime, err)
	}
	if card.EntityType != models.EntityTypeCard {
		t.Errorf("EntityType = %q, want %q", card.EntityType, models.EntityTypeCard)
	}
}

func TestCardUpdateAttrs(t *testing.T) {
	cfg := cardConfig()
	attrs := cfg.UpdateAttrs(models.UpdateCardRequest{
		Question:          "Updated?",
		TagIds:            []string{"t1"},
		PreviouslyCorrect: true,
	})

	if attrs["question"] != "Updated?" {
		t.Errorf("question = %v", attrs["question"])
	}
	if attrs["previously_correct"] != true {
		t.Errorf("previously_correct = %v", attrs["previously_correct"])
	}
	if _, ok := attrs["updated_date_time"]; !ok {
		t.Error("updated_date_time should be refreshed on update")
	}
	if _, ok := attrs["last_accessed_date_time"]; ok {
		t.Error("last_accessed_date_time should be omitted when empty")
	}

	attrs = cfg.UpdateAttrs(models.UpdateCardRequest{Question: "q", LastAccessedDateTime: "2026-07-06T00:00:00Z"})
	if attrs["last_accessed_date_time"] != "2026-07-06T00:00:00Z" {
		t.Errorf("last_accessed_date_time = %v", attrs["last_accessed_date_time"])
	}
}

func TestSharedCardIDIndexEntitiesFilterByEntityType(t *testing.T) {
	sections := NewCardAnswerSectionRepository(&Store{}).(*DynamoRepository[models.CardAnswerSection, models.CreateCardAnswerSectionRequest, models.UpdateCardAnswerSectionRequest])
	images := NewCardQuestionImageRepository(&Store{}).(*DynamoRepository[models.CardQuestionImage, models.CreateCardQuestionImageRequest, models.UpdateCardQuestionImageRequest])

	// Both entity types live on card_id-index; without the filter their
	// List results would mix.
	if !sections.cfg.FilterByEntityType || sections.cfg.ListIndex != IndexCardID {
		t.Errorf("sections config = %+v", sections.cfg)
	}
	if !images.cfg.FilterByEntityType || images.cfg.ListIndex != IndexCardID {
		t.Errorf("images config = %+v", images.cfg)
	}
}
