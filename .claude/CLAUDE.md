# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Run all tests
go test ./...

# Run a single package's tests
go test ./internal/httpapi/...

# Vet code
go vet ./...

# Tidy dependencies
go mod tidy

# Run the API locally (needs AWS credentials + env vars, see README)
go run ./cmd/server -addr :8080

# Build/deploy for AWS (via SAM; uses Makefile's build-FlashcardFunction)
sam build && sam deploy
```

## Architecture

Go backend for a flashcard app. The core is a standard `http.Handler`; `cmd/lambda` wraps it with `aws-lambda-go-api-proxy/httpadapter` for API Gateway, `cmd/server` serves it locally.

**Request flow:**
1. `cmd/lambda/main.go` / `cmd/server/main.go` — entry points (`package main`)
2. `internal/app/app.go` — wires DynamoDB/S3 implementations into the router; the only place implementations are chosen
3. `internal/httpapi/` — `ServeMux` routing (Go 1.22 method patterns), CORS middleware (headers on ALL responses incl. errors), generic `Resource[T, C, U]` CRUD handlers, validator.v9 request validation
4. `internal/persistence/` — `Repository[T, C, U]` interface + generic DynamoDB implementation; per-entity configs (GSI names, entity constructors, update-attribute maps) live in `entities.go`
5. `internal/service/cascade.go` — cascading deletes (children before parent; S3 object deletes best-effort after record delete)
6. `internal/storage/` — `ImageStore` interface + S3 impl; deletes parse bucket+key from the stored image URL (legacy two-bucket data still works)

**Key design points:**
- Single DynamoDB table (PK `id`), all entities carry `entity_type`. List operations are GSI **Queries**, never Scans. GSIs: `entity_type-index`, `category_id-index`, `deck_id-index`, `card_id-index` (shared by answer sections + question images → needs `entity_type` filter), `card_answer_section_id-index`
- Config via env vars `DYNAMODB_TABLE` and `S3_BUCKET` (`internal/config`); single bucket with `question-images/` and `answer-images/` prefixes
- Tests use in-memory fakes from `internal/testutil` against the real router/services — no AWS needed
- `template.yaml` (SAM) defines the whole stack including API key auth; API contract details and migration notes are in README.md

**Data model hierarchy:** Category → Deck → Card (with AnswerSections → AnswerSectionImages, QuestionImages, and Tags)

**Module name:** `flashcard_lambda` (see `go.mod`)

## Rules

- Do not run `go build` after making code changes unless explicitly asked to (use `go vet ./...` / `go test ./...` to verify compilation).
