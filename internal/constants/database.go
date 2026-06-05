package constants

import "os"

func GetDBName() string {
	return os.Getenv("DYNAMODB_TABLE")
}
