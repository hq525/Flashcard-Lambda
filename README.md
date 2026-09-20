# Flashcard Lambda

Go backend for a single owner's private flashcard library. It runs on AWS Lambda behind API Gateway, with DynamoDB persistence and private S3 images. Both production and the loopback development server require a verified Cognito ID token with the `owner` group. Public signup is disabled.

**Already deployed?** Use the [security cutover runbook](docs/security-deployment-2026-09-20.md). This release changes authentication and image storage together; deploying only one repository interrupts the old client.

## Architecture

`API Gateway → Lambda → httpadapter → authenticated http.Handler → DynamoDB / private S3`

- `internal/auth/` independently verifies RS256 signatures, issuer, audience, expiry, token type and owner group.
- `internal/httpapi/` handles strict JSON validation, typed parent checks, CORS and raw image uploads.
- `internal/persistence/` implements typed repositories and bounded GSI queries. Administrative migration commands separately use bounded scans.
- `internal/service/` cascades deletes through child records and managed image objects.
- `internal/storage/` validates image bytes, writes immutable objects and signs five-minute downloads.
- `internal/app/` wires AWS clients into the same handler used by Lambda and local development.

## Data and API

All entities share a DynamoDB table partitioned by `id`, with an `entity_type` discriminator. Existing IDs, card content and review history are retained.

| Resource | List filter | Relationship |
|---|---|---|
| `/categories`, `/category` | — | Contains decks |
| `/decks`, `/deck` | `categoryId` | Contains cards |
| `/tags`, `/tag` | — | Associated with cards |
| `/cards`, `/card` | `deckId` | Contains answer sections and question images |
| `/card-answer-sections`, `/card-answer-section` | `cardId` | Contains answer images |
| `/card-question-images`, `/card-question-image` | `cardId` | Private question image |
| `/card-answer-section-images`, `/card-answer-section-image` | `cardAnswerSectionId` | Private answer image |

Use `GET /<plural>` for lists and `GET/PUT/DELETE /<singular>?id=...` for individual records. Most `POST /<singular>` requests accept JSON. Deletes cascade to descendants. The GSIs are `entity_type-index`, `category_id-index`, `deck_id-index`, `card_id-index`, and `card_answer_section_id-index`.

All routes require `Authorization: Bearer <Cognito ID token>`. API keys grant no access. The backend verifies identity even without API Gateway. CORS accepts one configured frontend origin; it is not authentication.

Image creation uses raw bytes:

- `POST /card-question-image?cardId=<id>&sequenceNumber=1`
- `POST /card-answer-section-image?cardAnswerSectionId=<id>&sequenceNumber=1`

Send `Content-Type: image/png`, `image/jpeg`, `image/gif`, or `image/webp`. Input is limited to **4 MiB**, dimensions to 8192 per axis and 16 million pixels total. The server decodes and re-encodes each image as JPEG or static PNG, removing metadata and trailing content. Animation and EXIF orientation metadata are not preserved. Normalized output is limited to 10 MiB. An atomic UTC-day budget permits 100 uploads and 100 MiB of normalized data; failed storage attempts may consume budget. Exceeding the budget returns `429`.

Image DTOs contain a five-minute signed `imageURL`, never a storage key. Image downloads permit only short browser caching (`private, max-age=60, must-revalidate`); API JSON remains `no-store`. Signed response headers apply this policy to existing managed images too, without rewriting their metadata. Image PUT accepts only `sequenceNumber`; client URLs cannot select an S3 object. Unmigrated legacy images return an empty URL and cannot be deleted until migrated. `GET /presigned-url` returns `410`; old clients must reload the updated frontend.

Responses are JSON: `201` on create, `404` for absent or wrong-type IDs, `400` for invalid parameters/parents, `422` for malformed or unknown JSON fields. Requests reject trailing JSON. Generic JSON bodies are limited to 128 KiB (reviews: 4 KiB), and IDs to 128 bytes. Lists fail explicitly with `413` above 1000 items, 50 query pages or 4 MiB; they never silently truncate. Successful responses are also limited to 4 MiB, with an additional escaped-body check below Lambda's 6 MiB proxy-envelope limit. Large libraries need pagination before raising these limits. Responses use `no-store`; requests have a 25-second context deadline.

## Spaced repetition

Reviews use **FSRS 6**, via the pinned `go-fsrs/v4 v4.0.0` library. The server uses default model weights, a 90% desired retention target, and one ten-minute learning/relearning step. Interval fuzz is disabled so previews and submissions use the same calculation. Successful reviews can grow beyond the former 16-day limit. Early reviews use FSRS's elapsed-time and same-day memory rules rather than advancing a fixed box.

The study screen offers **Again** (forgot or incorrect), **Hard** (correct with effort), **Good** (correct), and **Easy** (correct with ease). Each choice previews its next interval. Short learning/relearning steps return to the same session when due; if there is nothing else to study, a countdown and **Finish now** let the user wait or leave with progress saved.

| Method and route | Request / response |
|---|---|
| `GET /card-review-options?cardId=…` | `{cardId, revision, generatedAt, options:[{rating,dueAt,intervalSeconds,state}]}`; read only |
| `POST /card-review?cardId=…` | Body `{reviewId,rating,expectedRevision}`; returns `{card,review}` |
| `GET /card-reviews?cardId=…` | Review history in revision order; `[]` before the first review |

Ratings are lowercase `again`, `hard`, `good`, `easy`. `expectedRevision` is required (zero for cards without FSRS history), and `reviewId` is a unique identifier for one attempt, reused unchanged on network retries. A stale revision or reuse of an ID for a different rating returns `409`. The browser supplies neither timestamps nor scheduling state. Preview intervals are estimates at `generatedAt`; a later submission uses the actual server review time.

Cards store `schedule` (due time, difficulty, stability, stage, counts and last review) and `reviewRevision`. Each actual review atomically updates that state and creates an immutable `card_review` entity with the rating, server timestamp and before/after schedules. Content edits preserve FSRS state, and reviews do not rewrite question text or tags. History is listed through the existing `card_id-index` (eventually consistent); internal history links allow card deletion to clean up reviews with consistent reads, even immediately after saving. Failed deletion can be retried; while its cleanup is pending, the card rejects new reviews.

### Migration and rollout

Deploy the backend before the frontend. No table/index migration, bulk backfill or mass rescheduling is required. Existing cards without `schedule` keep their original last-review-plus-Leitner-interval due date until their next real review. Invalid or future legacy timestamps are treated as new and due now.

On that first review, the previous interval provides a conservative starting stability estimate, with neutral difficulty; legacy review counts and ratings are not invented. FSRS counts and recorded history begin with that actual review. This uses default FSRS weights, not a model trained on personal history; the saved records allow future parameter fitting. Existing card content is unchanged. The separate security migration replaces public image delivery with signed URLs while preserving image IDs and source objects.

The old general card PUT remains compatible for cards that have not migrated. Once a card has FSRS state, PUT only changes card content; clients must use the review endpoint to record recall and change scheduling.

## Environment and development

Use Go **1.26.8 or newer**. Tests use mocks and need no AWS credentials. Running the server requires AWS credentials and an owner login against a development Cognito pool.

| Variable | Meaning |
|---|---|
| `DYNAMODB_TABLE` | DynamoDB table name |
| `S3_BUCKET` | Private bucket for `images/<image-id>.png` or `.jpg` |
| `AUTH_ISSUER` | Cognito user-pool issuer, not the hosted login domain |
| `AUTH_CLIENT_ID` | Public Cognito app client ID |
| `ALLOWED_ORIGIN` | Exact frontend origin, without trailing slash |
| `AWS_REGION` | AWS region for local AWS clients |

```bash
go test ./...
go vet ./...
cp .env.example .env
# Fill in isolated development resource and authentication settings.
make run
```

The local server binds `127.0.0.1:8080`, rejects non-loopback addresses, and enforces the same JWT authentication. There is no development auth bypass. Register the development frontend callback and logout URL in its own Cognito app client. Environment files are ignored except `.env.example`.

## Infrastructure and deployment

`template.yaml` retains the existing table and bucket, adds Cognito with admin-only account creation, protects API Gateway, and configures the backend to require the `owner` group. Only assign that group to the intended single owner. The bucket blocks public access and requires TLS. DynamoDB point-in-time recovery and S3 versioning protect future changes; old noncurrent object versions expire after 30 days. Backups and retained versions incur storage charges.

The Lambda role has typed database operations and access only to this bucket's `images/*` prefix. API throttling and Lambda concurrency limit traffic; they are not guaranteed billing caps. Paid method-level CloudWatch metrics are disabled; ordinary stage metrics remain available. The 512 MiB function allocation supports bounded image decoding; recovery protection and upload validation remain enabled. Administrative migration permissions are separate from the runtime role.

For an existing deployment, follow the [backup, maintenance and cutover procedure](docs/security-deployment-2026-09-20.md), including frontend environment changes and owner provisioning. `FrontendOrigin` is a required stack parameter. A production build and deployment are explicit rollout steps, not performed by local tests.

```bash
# Syntax/infrastructure validation; does not deploy.
sam validate --lint
# Build/deploy only during the reviewed rollout:
sam build
sam deploy --stack-name flashcard-prod --region ap-southeast-1 --resolve-s3 --capabilities CAPABILITY_IAM --parameter-overrides StageName=prod FrontendOrigin=https://main.d21qooye31nta2.amplifyapp.com
```

For older manually created tables, the separate `cmd/backfill` command adds missing `entity_type` attributes (dry run by default). Complete and verify that schema backfill before media migration; image migration deliberately selects typed image records only. Do not restore the old public URL deletion behavior or grant the Lambda legacy-bucket access.

```bash
# Metadata validation only: no writes and no S3 object reads.
go run ./cmd/migrate-media --table flash-card-app-prod --bucket flash-card-app-media-prod --source-buckets flash-card-app-media-prod --region ap-southeast-1
```

Migration requires explicit `--apply` to write, validates source buckets/paths, copies and normalizes images, and conditionally attaches immutable keys. Original objects and URLs remain for recovery; they must be private. See the runbook before applying.

## Security evidence

The [original audit](docs/security-audit-2026-09-20.md) describes the vulnerable baseline. The [remediation record](docs/security-remediation-2026-09-20.md) tracks implemented fixes, validation and outstanding production rollout. Historical exploit probes are not the current regression suite; normal `go test ./...` runs the new security tests.
