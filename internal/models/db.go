package models

type Category struct {
	Id          string `json:"id" dynamodbav:"id"`
	EntityType  string `json:"entityType" dynamodbav:"entity_type"`
	Name        string `json:"name" dynamodbav:"name"`
	Description string `json:"description" dynamodbav:"description"`
}

type Deck struct {
	Id          string `json:"id" dynamodbav:"id"`
	CategoryId  string `json:"categoryID" dynamodbav:"category_id"`
	Name        string `json:"name" dynamodbav:"name"`
	Description string `json:"description" dynamodbav:"description"`
}

type Card struct {
	Id                   string   `json:"id" dynamodbav:"id"` // Must Correspond with DynamoDB Table Partition Key
	DeckId               string   `json:"deckID" dynamodbav:"deck_id"`
	TagIds               []string `json:"tags" dynamodbav:"tag_ids"`
	Question             string   `json:"question" dynamodbav:"question"`
	CreatedDateTime      string   `json:"createdDateTime" dynamodbav:"created_date_time"`
	UpdatedDateTime      string   `json:"updatedDateTime" dynamodbav:"updated_date_time"`
	LastAccessedDateTime string   `json:"lastAccessedDateTime" dynamodbav:"last_accessed_date_time"`
	PreviouslyCorrect    bool     `json:"memorized" dynamodbav:"previously_correct"`
}

type Tag struct {
	Id          string `json:"id" dynamodbav:"id"`
	EntityType  string `json:"entityType" dynamodbav:"entity_type"`
	Name        string `json:"name" dynamodbav:"name"`
	Description string `json:"description" dynamodbav:"description"`
}

type CardQuestionImage struct {
	Id              string `json:"id" dynamodbav:"id"`
	CardId          string `json:"cardID" dynamodbav:"card_id"`
	SequenceNumber  uint16 `json:"sequenceNumber" dynamodbav:"sequence_number"`
	CreatedDateTime string `json:"createdDateTime" dynamodbav:"created_date_time"`
	ImageURL        string `json:"imageURL" dynamodbav:"image_url"`
}

type CardAnswerSection struct {
	Id              string `json:"id" dynamodbav:"id"`
	CardId          string `json:"cardID" dynamodbav:"card_id"`
	SequenceNumber  uint16 `json:"sequenceNumber" dynamodbav:"sequence_number"`
	Title           string `json:"title" dynamodbav:"title"`
	Answer          string `json:"answer" dynamodbav:"answer"`
	CreatedDateTime string `json:"createdDateTime" dynamodbav:"created_date_time"`
	UpdatedDateTime string `json:"updatedDateTime" dynamodbav:"updated_date_time"`
}

type CardAnswerSectionImage struct {
	Id                  string `json:"id" dynamodbav:"id"`
	CardAnswerSectionId string `json:"cardAnswerSectionID" dynamodbav:"card_answer_section_id"`
	SequenceNumber      uint16 `json:"sequenceNumber" dynamodbav:"sequence_number"`
	CreatedDateTime     string `json:"createdDateTime" dynamodbav:"created_date_time"`
	ImageURL            string `json:"imageURL" dynamodbav:"image_url"`
}
