package models

type Card struct {
	Id                   string   `json:"id" dynamodbav:"id"` // Must Correspond with DynamoDB Table Partition Key
	SetId                string   `json:"setID" dynamodbav:"set_id"`
	Tags                 []string `json:"tags" dynamodbsv:"tags"`
	Title                string   `json:"title" dynamodbav:"title"`
	Question             string   `json:"question" dynamodbav:"question"`
	QuestionPhotoURLs    []string `json:"questionPhotoURLs" dynamodbav:"question_photo_urls"`
	AnswerSectionIDs     []string `json:"answerSectionIDs" dynamodbav:"answer_section_ids"`
	CreatedDateTime      string   `json:"createdDateTime" dynamodbav:"created_date_time"`
	UpdatedDateTime      string   `json:"updatedDateTime" dynamodbav:"updated_date_time"`
	LastAccessedDateTime string   `json:"lastAccessedDateTime" dynamodbav:"last_accessed_date_time"`
	Memorized            bool     `json:"memorized" dynamodbav:"memorized"`
}

type AnswerSection struct {
	Id              string   `json:"id" dynamodbav:"id"`
	TypeId          string   `json:"typeID" dynamodbav:"type_id"`
	Answer          string   `json:"answer" dynamodbav:"answer"`
	AnswerPhotoURLs []string `json:"answerPhotoURLs" dynamodbav:"answer_photo_urls"`
}

type AnswerSectionType struct {
	Id   string `json:"id" dynamodbav:"id"`
	Name string `json:"name" dynamodbav:"name"`
}

type Tag struct {
	Id          string `json:"id" dynamodbav:"id"`
	Name        string `json:"name" dynamodbav:"name"`
	Description string `json:"description" dynamodbav:"description"`
}

type Set struct {
	Id          string `json:"id" dynamodbav:"id"`
	CategoryId  string `json:"categoryID" dynamodbav:"category_id"`
	Name        string `json:"name" dynamodbav:"name"`
	Description string `json:"description" dynamodbav:"description"`
}

type Category struct {
	Id          string `json:"id" dynamodbav:"id"`
	Name        string `json:"name" dynamodbav:"name"`
	Description string `json:"description" dynamodbav:"description"`
}
