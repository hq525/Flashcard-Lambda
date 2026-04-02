# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build Lambda binary (Linux/ARM64 for AWS)
GOOS=linux GOARCH=arm64 go build -o bootstrap ./cmd/lambda/

# Build for local/testing
go build ./...

# Run all tests
go test ./...

# Run a single package's tests
go test ./internal/controllers/...

# Tidy dependencies
go mod tidy

# Vet code
go vet ./...
```

## Architecture

This is a Go AWS Lambda backend for a flashcard application, routed via API Gateway.

**Request flow:**
1. `cmd/lambda/main.go` — Lambda entry point; initializes DynamoDB client and starts the handler
2. `cmd/lambda/api/methods.go` — Routes by HTTP method (`ProcessGet` / `ProcessPost`) and path
3. `internal/controllers/` — Business logic per resource
4. DynamoDB via `aws-sdk-go-v2`

**Key design points:**
- Environment selection (prod vs dev) is inferred from the API Gateway request context stage: `flash-card-app` (prod) vs `flash-card-app-dev` (dev) — see `internal/constants/db.go`
- CORS headers (`*` origin) are added to all responses via `internal/constants/response.go`
- Input validation uses `gopkg.in/go-playground/validator.v9` struct tags on request models
- `internal/persistence/` holds DAO interfaces (currently defined but not wired up — controllers call DynamoDB directly)

**Data model hierarchy:** Category → Set → Card (with AnswerSections and Tags)

**Module name:** `flashcard_lambda` (see `go.mod`)

## Rules

- Do not run `go build` after making code changes unless explicitly asked to.
