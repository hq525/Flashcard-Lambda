package persistence

import (
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gorm.io/gorm"
)

type ICategoryDataAccessObject interface {
}

type CategoryDataAccessObject struct {
	db dynamodb.Client
}

func NewCategoryDataAccessObject(db *gorm.DB) ICategoryDataAccessObject {
	return &CategoryDataAccessObject{
		db: db,
	}
}
