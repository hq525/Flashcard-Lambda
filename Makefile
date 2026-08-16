# Used by `sam build` (BuildMethod: makefile in template.yaml).
build-FlashcardFunction:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o $(ARTIFACTS_DIR)/bootstrap ./cmd/lambda

# Local dev server, loading DYNAMODB_TABLE/S3_BUCKET from .env (see .env.example).
run:
	set -a && . ./.env && set +a && go run ./cmd/server -addr :8080
