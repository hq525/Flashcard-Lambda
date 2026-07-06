package persistence

import (
	"time"

	"github.com/google/uuid"

	"flashcard_lambda/internal/models"
)

// GSI names. All index keys are pre-existing item attributes, so creating
// these indexes requires no data migration (see README "Migration").
const (
	IndexEntityType = "entity_type-index"
	IndexCategoryID = "category_id-index"
	IndexDeckID     = "deck_id-index"
	IndexCardID     = "card_id-index"
	IndexSectionID  = "card_answer_section_id-index"
)

func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func NewCategoryRepository(s *Store) Repository[models.Category, models.CreateCategoryRequest, models.UpdateCategoryRequest] {
	return NewDynamoRepository(s, EntityConfig[models.Category, models.CreateCategoryRequest, models.UpdateCategoryRequest]{
		EntityType: models.EntityTypeCategory,
		ListIndex:  IndexEntityType,
		ListKey:    "entity_type",
		New: func(req models.CreateCategoryRequest) models.Category {
			return models.Category{
				Id:          uuid.NewString(),
				EntityType:  models.EntityTypeCategory,
				Name:        req.Name,
				Description: req.Description,
			}
		},
		UpdateAttrs: func(req models.UpdateCategoryRequest) map[string]any {
			return map[string]any{
				"name":        req.Name,
				"description": req.Description,
			}
		},
	})
}

func NewTagRepository(s *Store) Repository[models.Tag, models.CreateTagRequest, models.UpdateTagRequest] {
	return NewDynamoRepository(s, EntityConfig[models.Tag, models.CreateTagRequest, models.UpdateTagRequest]{
		EntityType: models.EntityTypeTag,
		ListIndex:  IndexEntityType,
		ListKey:    "entity_type",
		New: func(req models.CreateTagRequest) models.Tag {
			return models.Tag{
				Id:          uuid.NewString(),
				EntityType:  models.EntityTypeTag,
				Name:        req.Name,
				Description: req.Description,
			}
		},
		UpdateAttrs: func(req models.UpdateTagRequest) map[string]any {
			return map[string]any{
				"name":        req.Name,
				"description": req.Description,
			}
		},
	})
}

func NewDeckRepository(s *Store) Repository[models.Deck, models.CreateDeckRequest, models.UpdateDeckRequest] {
	return NewDynamoRepository(s, EntityConfig[models.Deck, models.CreateDeckRequest, models.UpdateDeckRequest]{
		EntityType: models.EntityTypeDeck,
		ListIndex:  IndexCategoryID,
		ListKey:    "category_id",
		New: func(req models.CreateDeckRequest) models.Deck {
			return models.Deck{
				Id:          uuid.NewString(),
				EntityType:  models.EntityTypeDeck,
				CategoryId:  req.CategoryId,
				Name:        req.Name,
				Description: req.Description,
			}
		},
		UpdateAttrs: func(req models.UpdateDeckRequest) map[string]any {
			return map[string]any{
				"name":        req.Name,
				"description": req.Description,
			}
		},
	})
}

func NewCardRepository(s *Store) Repository[models.Card, models.CreateCardRequest, models.UpdateCardRequest] {
	return NewDynamoRepository(s, EntityConfig[models.Card, models.CreateCardRequest, models.UpdateCardRequest]{
		EntityType: models.EntityTypeCard,
		ListIndex:  IndexDeckID,
		ListKey:    "deck_id",
		New: func(req models.CreateCardRequest) models.Card {
			ts := now()
			return models.Card{
				Id:              uuid.NewString(),
				EntityType:      models.EntityTypeCard,
				DeckId:          req.DeckId,
				TagIds:          req.TagIds,
				Question:        req.Question,
				CreatedDateTime: ts,
				UpdatedDateTime: ts,
			}
		},
		UpdateAttrs: func(req models.UpdateCardRequest) map[string]any {
			attrs := map[string]any{
				"question":           req.Question,
				"tag_ids":            req.TagIds,
				"previously_correct": req.PreviouslyCorrect,
				"updated_date_time":  now(),
			}
			if req.LastAccessedDateTime != "" {
				attrs["last_accessed_date_time"] = req.LastAccessedDateTime
			}
			return attrs
		},
	})
}

func NewCardAnswerSectionRepository(s *Store) Repository[models.CardAnswerSection, models.CreateCardAnswerSectionRequest, models.UpdateCardAnswerSectionRequest] {
	return NewDynamoRepository(s, EntityConfig[models.CardAnswerSection, models.CreateCardAnswerSectionRequest, models.UpdateCardAnswerSectionRequest]{
		EntityType: models.EntityTypeCardAnswerSection,
		ListIndex:  IndexCardID,
		ListKey:    "card_id",
		// card_id is shared with question images, so List must filter.
		FilterByEntityType: true,
		New: func(req models.CreateCardAnswerSectionRequest) models.CardAnswerSection {
			ts := now()
			return models.CardAnswerSection{
				Id:              uuid.NewString(),
				EntityType:      models.EntityTypeCardAnswerSection,
				CardId:          req.CardId,
				SequenceNumber:  req.SequenceNumber,
				Title:           req.Title,
				Answer:          req.Answer,
				CreatedDateTime: ts,
				UpdatedDateTime: ts,
			}
		},
		UpdateAttrs: func(req models.UpdateCardAnswerSectionRequest) map[string]any {
			return map[string]any{
				"sequence_number":   req.SequenceNumber,
				"title":             req.Title,
				"answer":            req.Answer,
				"updated_date_time": now(),
			}
		},
	})
}

func NewCardQuestionImageRepository(s *Store) Repository[models.CardQuestionImage, models.CreateCardQuestionImageRequest, models.UpdateCardQuestionImageRequest] {
	return NewDynamoRepository(s, EntityConfig[models.CardQuestionImage, models.CreateCardQuestionImageRequest, models.UpdateCardQuestionImageRequest]{
		EntityType: models.EntityTypeCardQuestionImage,
		ListIndex:  IndexCardID,
		ListKey:    "card_id",
		// card_id is shared with answer sections, so List must filter.
		FilterByEntityType: true,
		New: func(req models.CreateCardQuestionImageRequest) models.CardQuestionImage {
			return models.CardQuestionImage{
				Id:              uuid.NewString(),
				EntityType:      models.EntityTypeCardQuestionImage,
				CardId:          req.CardId,
				SequenceNumber:  req.SequenceNumber,
				ImageURL:        req.ImageURL,
				CreatedDateTime: now(),
			}
		},
		UpdateAttrs: func(req models.UpdateCardQuestionImageRequest) map[string]any {
			return map[string]any{
				"sequence_number": req.SequenceNumber,
				"image_url":       req.ImageURL,
			}
		},
	})
}

func NewCardAnswerSectionImageRepository(s *Store) Repository[models.CardAnswerSectionImage, models.CreateCardAnswerSectionImageRequest, models.UpdateCardAnswerSectionImageRequest] {
	return NewDynamoRepository(s, EntityConfig[models.CardAnswerSectionImage, models.CreateCardAnswerSectionImageRequest, models.UpdateCardAnswerSectionImageRequest]{
		EntityType: models.EntityTypeCardAnswerSectionImage,
		ListIndex:  IndexSectionID,
		ListKey:    "card_answer_section_id",
		New: func(req models.CreateCardAnswerSectionImageRequest) models.CardAnswerSectionImage {
			return models.CardAnswerSectionImage{
				Id:                  uuid.NewString(),
				EntityType:          models.EntityTypeCardAnswerSectionImage,
				CardAnswerSectionId: req.CardAnswerSectionId,
				SequenceNumber:      req.SequenceNumber,
				ImageURL:            req.ImageURL,
				CreatedDateTime:     now(),
			}
		},
		UpdateAttrs: func(req models.UpdateCardAnswerSectionImageRequest) map[string]any {
			return map[string]any{
				"sequence_number": req.SequenceNumber,
				"image_url":       req.ImageURL,
			}
		},
	})
}
