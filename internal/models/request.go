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
