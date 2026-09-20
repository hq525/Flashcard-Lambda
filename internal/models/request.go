package models

type CreateCategoryRequest struct {
	Name        string `json:"name" validate:"required,max=256"`
	Description string `json:"description" validate:"max=8192"`
}

type UpdateCategoryRequest struct {
	Name        string `json:"name" validate:"required,max=256"`
	Description string `json:"description" validate:"max=8192"`
}

type CreateDeckRequest struct {
	CategoryId  string `json:"categoryID" validate:"required,max=128"`
	Name        string `json:"name" validate:"required,max=256"`
	Description string `json:"description" validate:"max=8192"`
}

type UpdateDeckRequest struct {
	Name        string `json:"name" validate:"required,max=256"`
	Description string `json:"description" validate:"max=8192"`
}

type CreateTagRequest struct {
	Name        string `json:"name" validate:"required,max=256"`
	Description string `json:"description" validate:"max=8192"`
}

type UpdateTagRequest struct {
	Name        string `json:"name" validate:"required,max=256"`
	Description string `json:"description" validate:"max=8192"`
}

type CreateCardRequest struct {
	DeckId   string   `json:"deckID" validate:"required,max=128"`
	Question string   `json:"question" validate:"required,max=32768"`
	TagIds   []string `json:"tags" validate:"max=100,dive,required,max=128"`
}

type UpdateCardRequest struct {
	Question             string   `json:"question" validate:"required,max=32768"`
	TagIds               []string `json:"tags" validate:"max=100,dive,required,max=128"`
	PreviouslyCorrect    bool     `json:"memorized"`
	LastAccessedDateTime string   `json:"lastAccessedDateTime" validate:"max=64"`
	// 0 = leave the stored box unchanged (non-study edits omit it).
	LeitnerBox uint8 `json:"leitnerBox" validate:"max=5"`
}

type CreateCardAnswerSectionRequest struct {
	CardId         string `json:"cardID" validate:"required,max=128"`
	SequenceNumber uint16 `json:"sequenceNumber" validate:"required"`
	Title          string `json:"title" validate:"max=256"`
	Answer         string `json:"answer" validate:"max=32768"`
}

type UpdateCardAnswerSectionRequest struct {
	SequenceNumber uint16 `json:"sequenceNumber" validate:"required"`
	Title          string `json:"title" validate:"max=256"`
	Answer         string `json:"answer" validate:"max=32768"`
}

// Image create inputs are constructed only by the validated binary upload handler.
// The identifier and storage key cannot be supplied through JSON.
type CreateCardQuestionImageRequest struct {
	Id             string `json:"-"`
	CardId         string `json:"cardID" validate:"required,max=128"`
	SequenceNumber uint16 `json:"sequenceNumber" validate:"required"`
	StorageKey     string `json:"-"`
}

type UpdateCardQuestionImageRequest struct {
	SequenceNumber uint16 `json:"sequenceNumber" validate:"required"`
}

type CreateCardAnswerSectionImageRequest struct {
	Id                  string `json:"-"`
	CardAnswerSectionId string `json:"cardAnswerSectionID" validate:"required,max=128"`
	SequenceNumber      uint16 `json:"sequenceNumber" validate:"required"`
	StorageKey          string `json:"-"`
}

type UpdateCardAnswerSectionImageRequest struct {
	SequenceNumber uint16 `json:"sequenceNumber" validate:"required"`
}
