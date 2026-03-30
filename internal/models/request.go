package models

type CreateCategoryRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}
