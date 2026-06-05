# Flashcard Lambda

Go AWS Lambda backend for a flashcard application, fronted by API Gateway with DynamoDB for persistence and S3 for image storage.

## Architecture

```
API Gateway → Lambda Handler → Router (methods.go) → Controllers → DynamoDB / S3
```

**Request flow:**
1. `cmd/lambda/main.go` — entry point; initializes AWS clients and starts the Lambda handler
2. `cmd/lambda/api/methods.go` — routes by HTTP method and path to the appropriate controller
3. `internal/controllers/` — business logic per resource
4. `internal/models/` — DynamoDB data models
5. `internal/persistence/` — DAO interfaces

**Environment variables:**

| Variable | Description |
|---|---|
| `DYNAMODB_TABLE` | DynamoDB table name (e.g. `flash-card-app` for prod, `flash-card-app-dev` for dev) |
| `S3_BUCKET` | S3 bucket for images (e.g. `flash-card-app-images` for prod, `flash-card-app-images-dev` for dev) |

Set these in the Lambda function configuration. AWS Lambda makes them available to the process at runtime via `os.Getenv`.

## Data Model

```
Category
└── Deck
    └── Card
        ├── CardAnswerSection (ordered by sequence_number)
        └── CardQuestionImage (ordered by sequence_number)

Tag  (associated with Cards via tag_ids)
```

## API Routes

| Method | Path | Description |
|---|---|---|
| GET | `/categories` | List all categories |
| GET | `/category` | Get a category by ID |
| POST | `/category` | Create a category |
| PUT | `/category` | Update a category |
| DELETE | `/category` | Delete a category |
| GET | `/decks` | List all decks |
| GET | `/deck` | Get a deck by ID |
| POST | `/deck` | Create a deck |
| PUT | `/deck` | Update a deck |
| DELETE | `/deck` | Delete a deck |
| GET | `/tags` | List all tags |
| GET | `/tag` | Get a tag by ID |
| POST | `/tag` | Create a tag |
| PUT | `/tag` | Update a tag |
| DELETE | `/tag` | Delete a tag |
| GET | `/card-answer-sections` | List answer sections for a card |
| GET | `/card-answer-section` | Get an answer section by ID |
| POST | `/card-answer-section` | Create an answer section |
| PUT | `/card-answer-section` | Update an answer section |
| DELETE | `/card-answer-section` | Delete an answer section |
| GET | `/card-question-images` | List question images for a card |
| GET | `/card-question-image` | Get a question image by ID |
| POST | `/card-question-image` | Create a question image record |
| PUT | `/card-question-image` | Update a question image record |
| DELETE | `/card-question-image` | Delete a question image record |
| GET | `/presigned-url` | Get a pre-signed S3 URL for image upload |

All responses include CORS headers (`Access-Control-Allow-Origin: *`).

## Configuration

The function requires two environment variables. Set them in the Lambda function configuration for each environment (prod and dev).

**AWS Console:**
1. Open the [Lambda console](https://console.aws.amazon.com/lambda) and select your function
2. Go to **Configuration → Environment variables → Edit**
3. Add the following key-value pairs and click **Save**

| Key | Example value |
|---|---|
| `DYNAMODB_TABLE` | `flash-card-app` |
| `S3_BUCKET` | `flash-card-app-images` |

**AWS CLI:**
```bash
aws lambda update-function-configuration \
  --function-name <your-function-name> \
  --environment "Variables={DYNAMODB_TABLE=flash-card-app,S3_BUCKET=flash-card-app-images}"
```

For the dev function, use the dev resource names:
```bash
aws lambda update-function-configuration \
  --function-name <your-dev-function-name> \
  --environment "Variables={DYNAMODB_TABLE=flash-card-app-dev,S3_BUCKET=flash-card-app-images-dev}"
```

**Local testing:**

Set the variables in your shell before running:
```bash
export DYNAMODB_TABLE=flash-card-app-dev
export S3_BUCKET=flash-card-app-images-dev
```

## Development

**Prerequisites:** Go 1.24+, AWS credentials configured

```bash
# Build Lambda binary (Linux/ARM64 for AWS deployment)
GOOS=linux GOARCH=arm64 go build -o bootstrap ./cmd/lambda/

# Build for local/testing
go build ./...

# Run tests
go test ./...

# Run tests for a single package
go test ./internal/controllers/...

# Tidy dependencies
go mod tidy

# Vet code
go vet ./...
```

## Project Structure

```
cmd/
  lambda/
    main.go          # Lambda entry point
    api/
      methods.go     # HTTP method and path routing
internal/
  constants/         # DB table names, S3 bucket names, CORS headers
  controllers/       # Business logic per resource
  models/            # DynamoDB data models and request structs
  persistence/       # DAO interfaces
  utils/             # Error helpers
```

## Dependencies

- [`aws-lambda-go`](https://github.com/aws/aws-lambda-go) — Lambda runtime and API Gateway event types
- [`aws-sdk-go-v2`](https://github.com/aws/aws-sdk-go-v2) — DynamoDB and S3 clients
- [`google/uuid`](https://github.com/google/uuid) — UUID generation for entity IDs
- [`go-playground/validator.v9`](https://github.com/go-playground/validator) — Request input validation
