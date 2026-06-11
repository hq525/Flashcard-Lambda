package models

type CreateCategoryRequest struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description"`
}

type UpdateCategoryRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type CreateDeckRequest struct {
	CategoryId  string `json:"categoryID" validate:"required"`
	Name        string `json:"name" validate:"required"`
	Description string `json:"description"`
}

type UpdateDeckRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type CreateTagRequest struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description"`
}

type UpdateTagRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type CreateCardAnswerSectionRequest struct {
	CardId         string `json:"cardID" validate:"required"`
	SequenceNumber uint16 `json:"sequenceNumber" validate:"required"`
	Title          string `json:"title"`
	Answer         string `json:"answer"`
}

type UpdateCardAnswerSectionRequest struct {
	SequenceNumber uint16 `json:"sequenceNumber"`
	Title          string `json:"title"`
	Answer         string `json:"answer"`
}

type CreateCardQuestionImageRequest struct {
	CardId         string `json:"cardID" validate:"required"`
	SequenceNumber uint16 `json:"sequenceNumber" validate:"required"`
	ImageURL       string `json:"imageURL" validate:"required"`
}

type UpdateCardQuestionImageRequest struct {
	SequenceNumber uint16 `json:"sequenceNumber"`
	ImageURL       string `json:"imageURL"`
}

type CreateCardAnswerSectionImageRequest struct {
	CardAnswerSectionId string `json:"cardAnswerSectionID" validate:"required"`
	SequenceNumber      uint16 `json:"sequenceNumber" validate:"required"`
	ImageURL            string `json:"imageURL" validate:"required"`
}

type UpdateCardAnswerSectionImageRequest struct {
	SequenceNumber uint16 `json:"sequenceNumber"`
	ImageURL       string `json:"imageURL"`
}
