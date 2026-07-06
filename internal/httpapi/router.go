package httpapi

import (
	"net/http"

	"flashcard_lambda/internal/models"
	"flashcard_lambda/internal/persistence"
	"flashcard_lambda/internal/service"
	"flashcard_lambda/internal/storage"
)

// Deps holds everything the router needs. Handlers depend only on these
// interfaces, so any Repository/ImageStore implementation can be wired in
// (DynamoDB+S3 in production, fakes in tests).
type Deps struct {
	Categories     persistence.Repository[models.Category, models.CreateCategoryRequest, models.UpdateCategoryRequest]
	Decks          persistence.Repository[models.Deck, models.CreateDeckRequest, models.UpdateDeckRequest]
	Tags           persistence.Repository[models.Tag, models.CreateTagRequest, models.UpdateTagRequest]
	Cards          persistence.Repository[models.Card, models.CreateCardRequest, models.UpdateCardRequest]
	Sections       persistence.Repository[models.CardAnswerSection, models.CreateCardAnswerSectionRequest, models.UpdateCardAnswerSectionRequest]
	QuestionImages persistence.Repository[models.CardQuestionImage, models.CreateCardQuestionImageRequest, models.UpdateCardQuestionImageRequest]
	SectionImages  persistence.Repository[models.CardAnswerSectionImage, models.CreateCardAnswerSectionImageRequest, models.UpdateCardAnswerSectionImageRequest]
	Images         storage.ImageStore
	Cascade        *service.Cascade
}

func NewRouter(d Deps) http.Handler {
	categories := &Resource[models.Category, models.CreateCategoryRequest, models.UpdateCategoryRequest]{
		Repo:     d.Categories,
		DeleteFn: d.Cascade.DeleteCategory,
	}
	decks := &Resource[models.Deck, models.CreateDeckRequest, models.UpdateDeckRequest]{
		Repo:      d.Decks,
		ListParam: "categoryId",
		DeleteFn:  d.Cascade.DeleteDeck,
	}
	tags := &Resource[models.Tag, models.CreateTagRequest, models.UpdateTagRequest]{
		Repo: d.Tags,
	}
	cards := &Resource[models.Card, models.CreateCardRequest, models.UpdateCardRequest]{
		Repo:      d.Cards,
		ListParam: "deckId",
		DeleteFn:  d.Cascade.DeleteCard,
	}
	sections := &Resource[models.CardAnswerSection, models.CreateCardAnswerSectionRequest, models.UpdateCardAnswerSectionRequest]{
		Repo:      d.Sections,
		ListParam: "cardId",
		DeleteFn:  d.Cascade.DeleteSection,
	}
	questionImages := &Resource[models.CardQuestionImage, models.CreateCardQuestionImageRequest, models.UpdateCardQuestionImageRequest]{
		Repo:      d.QuestionImages,
		ListParam: "cardId",
		DeleteFn:  d.Cascade.DeleteQuestionImage,
	}
	sectionImages := &Resource[models.CardAnswerSectionImage, models.CreateCardAnswerSectionImageRequest, models.UpdateCardAnswerSectionImageRequest]{
		Repo:      d.SectionImages,
		ListParam: "cardAnswerSectionId",
		DeleteFn:  d.Cascade.DeleteSectionImage,
	}

	mux := http.NewServeMux()
	registerResource(mux, "/categories", "/category", categories)
	registerResource(mux, "/decks", "/deck", decks)
	registerResource(mux, "/tags", "/tag", tags)
	registerResource(mux, "/cards", "/card", cards)
	registerResource(mux, "/card-answer-sections", "/card-answer-section", sections)
	registerResource(mux, "/card-question-images", "/card-question-image", questionImages)
	registerResource(mux, "/card-answer-section-images", "/card-answer-section-image", sectionImages)
	mux.HandleFunc("GET /presigned-url", getPresignedURL(d.Images))

	return withCORS(mux)
}

func registerResource[T any, C any, U any](mux *http.ServeMux, listPath, itemPath string, res *Resource[T, C, U]) {
	mux.HandleFunc("GET "+listPath, res.List)
	mux.HandleFunc("GET "+itemPath, res.Get)
	mux.HandleFunc("POST "+itemPath, res.Create)
	mux.HandleFunc("PUT "+itemPath, res.Update)
	mux.HandleFunc("DELETE "+itemPath, res.Delete)
}
