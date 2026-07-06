// Package service holds orchestration that spans multiple repositories —
// currently cascading deletes, so removing a parent cleans up its children
// and their S3 objects instead of leaving orphans.
package service

import (
	"context"
	"log"

	"flashcard_lambda/internal/models"
	"flashcard_lambda/internal/persistence"
	"flashcard_lambda/internal/storage"
)

type Cascade struct {
	Categories     persistence.Repository[models.Category, models.CreateCategoryRequest, models.UpdateCategoryRequest]
	Decks          persistence.Repository[models.Deck, models.CreateDeckRequest, models.UpdateDeckRequest]
	Cards          persistence.Repository[models.Card, models.CreateCardRequest, models.UpdateCardRequest]
	Sections       persistence.Repository[models.CardAnswerSection, models.CreateCardAnswerSectionRequest, models.UpdateCardAnswerSectionRequest]
	QuestionImages persistence.Repository[models.CardQuestionImage, models.CreateCardQuestionImageRequest, models.UpdateCardQuestionImageRequest]
	SectionImages  persistence.Repository[models.CardAnswerSectionImage, models.CreateCardAnswerSectionImageRequest, models.UpdateCardAnswerSectionImageRequest]
	Images         storage.ImageStore
}

// Children are deleted before their parent so a mid-cascade failure leaves
// the parent intact and the delete retryable. S3 object deletion happens
// after the record delete and is best-effort: an orphaned blob is cheaper
// than a record pointing at a missing image.

func (c *Cascade) DeleteCategory(ctx context.Context, id string) (*models.Category, error) {
	decks, err := c.Decks.List(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, deck := range decks {
		if _, err := c.DeleteDeck(ctx, deck.Id); err != nil {
			return nil, err
		}
	}
	return c.Categories.Delete(ctx, id)
}

func (c *Cascade) DeleteDeck(ctx context.Context, id string) (*models.Deck, error) {
	cards, err := c.Cards.List(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, card := range cards {
		if _, err := c.DeleteCard(ctx, card.Id); err != nil {
			return nil, err
		}
	}
	return c.Decks.Delete(ctx, id)
}

func (c *Cascade) DeleteCard(ctx context.Context, id string) (*models.Card, error) {
	images, err := c.QuestionImages.List(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, image := range images {
		if _, err := c.DeleteQuestionImage(ctx, image.Id); err != nil {
			return nil, err
		}
	}

	sections, err := c.Sections.List(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, section := range sections {
		if _, err := c.DeleteSection(ctx, section.Id); err != nil {
			return nil, err
		}
	}

	return c.Cards.Delete(ctx, id)
}

func (c *Cascade) DeleteSection(ctx context.Context, id string) (*models.CardAnswerSection, error) {
	images, err := c.SectionImages.List(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, image := range images {
		if _, err := c.DeleteSectionImage(ctx, image.Id); err != nil {
			return nil, err
		}
	}
	return c.Sections.Delete(ctx, id)
}

func (c *Cascade) DeleteQuestionImage(ctx context.Context, id string) (*models.CardQuestionImage, error) {
	image, err := c.QuestionImages.Delete(ctx, id)
	if err != nil || image == nil {
		return image, err
	}
	c.deleteObject(ctx, image.ImageURL)
	return image, nil
}

func (c *Cascade) DeleteSectionImage(ctx context.Context, id string) (*models.CardAnswerSectionImage, error) {
	image, err := c.SectionImages.Delete(ctx, id)
	if err != nil || image == nil {
		return image, err
	}
	c.deleteObject(ctx, image.ImageURL)
	return image, nil
}

func (c *Cascade) deleteObject(ctx context.Context, imageURL string) {
	if imageURL == "" {
		return
	}
	if err := c.Images.Delete(ctx, imageURL); err != nil {
		log.Printf("Failed to delete S3 object for %s (record already deleted): %v", imageURL, err)
	}
}
