# Flashcard Lambda

Go backend for a flashcard application. The app is a standard `http.Handler`; in production it runs on AWS Lambda behind API Gateway (via `aws-lambda-go-api-proxy`), with DynamoDB for persistence and S3 for image storage. Locally it runs as a plain HTTP server.

## Architecture

```
API Gateway → Lambda → httpadapter → http.Handler (router)
                                          │
                                    generic handlers
                                          │
                            Repository[T]        ImageStore
                            (DynamoDB impl)      (S3 impl)
```

**Request flow:**
1. `cmd/lambda/main.go` — Lambda entry point; `cmd/server/main.go` — identical app as a local HTTP server
2. `internal/app/app.go` — wires concrete implementations (DynamoDB, S3) into the router; the only place implementations are chosen
3. `internal/httpapi/` — routing (Go 1.22 `ServeMux` method patterns), CORS middleware, generic CRUD handlers, request validation
4. `internal/persistence/` — `Repository[T, C, U]` interface + one generic DynamoDB implementation; all list operations are GSI **Queries** (no table Scans)
5. `internal/service/cascade.go` — cascading deletes (category → decks → cards → sections/images, including S3 objects)
6. `internal/storage/` — `ImageStore` interface + S3 implementation (presigned uploads, deletes)

Handlers depend only on the `Repository` and `ImageStore` interfaces, so swapping DynamoDB or S3 for another backend means adding an implementation and changing one wiring function (`app.NewHandler`).

## Data Model

```
Category
└── Deck
    └── Card
        ├── CardAnswerSection (ordered by sequence_number)
        │   └── CardAnswerSectionImage (ordered by sequence_number)
        └── CardQuestionImage (ordered by sequence_number)

Tag  (associated with Cards via tag_ids)
```

All entities live in one DynamoDB table (partition key `id`) and carry an `entity_type` attribute. List operations query GSIs:

| Index | Partition key | Serves |
|---|---|---|
| `entity_type-index` | `entity_type` | list categories, list tags |
| `category_id-index` | `category_id` | decks by category |
| `deck_id-index` | `deck_id` | cards by deck |
| `card_id-index` | `card_id` | question images and answer sections by card (disambiguated by `entity_type` filter) |
| `card_answer_section_id-index` | `card_answer_section_id` | section images |

## API Routes

Every resource follows the same pattern: `GET /<plural>` (list), `GET/POST/PUT/DELETE /<singular>` (by `?id=`, body for POST/PUT).

| Resource | List param | Notes |
|---|---|---|
| `/categories`, `/category` | — | DELETE cascades to decks and below |
| `/decks`, `/deck` | `categoryId` | DELETE cascades to cards and below |
| `/tags`, `/tag` | — | |
| `/cards`, `/card` | `deckId` | DELETE cascades to sections and images |
| `/card-answer-sections`, `/card-answer-section` | `cardId` | DELETE cascades to section images |
| `/card-question-images`, `/card-question-image` | `cardId` | DELETE also removes the S3 object |
| `/card-answer-section-images`, `/card-answer-section-image` | `cardAnswerSectionId` | DELETE also removes the S3 object |

`GET /presigned-url?fileName=<name>&contentType=image/<type>[&imageType=answer]` returns `{presignedUrl, imageUrl}`. `contentType` is required, must be an `image/*` type, and is signed into the upload URL. Uploads land in `question-images/` or (with `imageType=answer`) `answer-images/` in the single image bucket.

**Response conventions:** JSON everywhere (errors are `{"message": "..."}`), CORS headers on every response including errors, `201` on create, `404` when an id doesn't exist, `400` for missing params/validation failures, `422` for malformed JSON. PUT bodies are validated like POST bodies (required fields enforced).

## Environment Variables

| Variable | Description |
|---|---|
| `DYNAMODB_TABLE` | DynamoDB table name |
| `S3_BUCKET` | S3 bucket for all card images (`question-images/` and `answer-images/` prefixes) |

## Development

**Prerequisites:** Go 1.24+, AWS credentials configured

```bash
# Run tests / vet
go test ./...
go vet ./...

# Run the API locally (uses your AWS credentials)
export DYNAMODB_TABLE=flash-card-app-dev
export S3_BUCKET=flash-card-app-media-dev
go run ./cmd/server -addr :8080
curl 'http://localhost:8080/categories'
```

## Deployment (SAM)

`template.yaml` defines the whole stack per stage: REST API (with API key auth), Lambda (`provided.al2023`/arm64), the DynamoDB table with all five GSIs, and the image bucket (browser-upload CORS + public read).

```bash
sam build
sam deploy --guided --parameter-overrides StageName=dev   # first time
sam deploy --parameter-overrides StageName=prod

# Get the API key the frontend must send as X-Api-Key
aws apigateway get-api-keys --include-values --query 'items[].{name:name,value:value}'
```

## Migrating an existing (manually created) deployment

The SAM stack creates **new** resources. To keep using existing tables/buckets instead, deploy only the function (or keep deploying the zip by hand) and:

**1. Add the GSIs to the existing table** (one at a time; wait for `IndexStatus: ACTIVE` between commands — DynamoDB builds one GSI at a time):

```bash
for idx in entity_type category_id deck_id card_id card_answer_section_id; do
  aws dynamodb update-table --table-name flash-card-app-dev \
    --attribute-definitions AttributeName=$idx,AttributeType=S \
    --global-secondary-index-updates "[{\"Create\":{\"IndexName\":\"$idx-index\",\"KeySchema\":[{\"AttributeName\":\"$idx\",\"KeyType\":\"HASH\"}],\"Projection\":{\"ProjectionType\":\"ALL\"}}}]"
  aws dynamodb wait table-exists --table-name flash-card-app-dev  # then poll describe-table for the index
done
```

**2. Backfill `entity_type`** on legacy items (required for answer sections and question images, which share `card_id-index`):

```bash
go run ./cmd/backfill -table flash-card-app-dev          # dry run, prints what it would set
go run ./cmd/backfill -table flash-card-app-dev -apply
```

**3. Bucket consolidation:** new uploads go to the single `S3_BUCKET` under `question-images/`/`answer-images/`. Existing images keep working — their full URL is stored on the record, and deletes parse bucket+key from that URL — but the Lambda role needs `s3:DeleteObject` on the legacy buckets (the template includes this; remove once legacy images are gone). `S3_ANSWER_IMAGE_BUCKET` is no longer read.

**4. Breaking API changes for the frontend:**
- `/presigned-url` now requires `contentType` (an `image/*` value) and the upload PUT must send the same `Content-Type` header
- With the SAM stack, every request must send an `X-Api-Key` header
- `GET /<entity>?id=missing` returns `404` instead of `200` with `null`; lists return `[]` instead of `null`; errors are JSON

## Project Structure

```
cmd/
  lambda/       # Lambda entry point (package main → bootstrap binary)
  server/       # local HTTP server entry point
  backfill/     # one-time entity_type migration
internal/
  app/          # dependency wiring (choose implementations here)
  config/       # environment variable loading
  httpapi/      # router, CORS middleware, generic CRUD handlers, presign handler
  models/       # entities and request structs (validator tags)
  persistence/  # Repository interface, generic DynamoDB store, per-entity configs
  service/      # cascading deletes
  storage/      # ImageStore interface, S3 implementation
  testutil/     # in-memory fakes for Repository and ImageStore
template.yaml   # SAM stack (API, Lambda, table + GSIs, bucket)
Makefile        # sam build target
```

## Dependencies

- [`aws-lambda-go`](https://github.com/aws/aws-lambda-go) — Lambda runtime
- [`aws-lambda-go-api-proxy`](https://github.com/awslabs/aws-lambda-go-api-proxy) — API Gateway events ↔ `http.Handler`
- [`aws-sdk-go-v2`](https://github.com/aws/aws-sdk-go-v2) — DynamoDB and S3 clients
- [`google/uuid`](https://github.com/google/uuid) — entity IDs
- [`go-playground/validator.v9`](https://github.com/go-playground/validator) — request validation
