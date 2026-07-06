# Flashcard Lambda Review Improvements Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Note:** This plan is executed inline in a non-interactive session; tasks are condensed (interfaces + verify commands rather than full code listings) because the plan author and executor are the same session. Nothing is committed until the user asks.

**Goal:** Implement all findings from the 2026-07-06 code review: deployability fix, CORS on all responses, auth-ready IaC, Query-over-Scan via GSIs, cascade deletes, update validation, single bucket, and a DI/`http.Handler` architecture with generic DAOs and tests.

**Architecture:** The app becomes a standard `http.Handler` (Go 1.22 `ServeMux` method patterns) wrapped by `aws-lambda-go-api-proxy/httpadapter` for Lambda and `http.ListenAndServe` for local dev. Handlers depend on a generic `Repository[T, C, U]` interface and an `ImageStore` interface, wired once in `internal/app`. DynamoDB access goes through one generic store using GSI Queries instead of Scans.

**Tech Stack:** Go 1.24, aws-sdk-go-v2, aws-lambda-go, aws-lambda-go-api-proxy, validator.v9, SAM.

## Global Constraints

- Module name stays `flashcard_lambda`; Go 1.24.
- API routes and query param names must not change (frontend compat): `id`, `categoryId`, `cardId`, `cardAnswerSectionId`, `fileName`, `imageType`. New `/cards` + `/card` routes use `deckId`.
- Env vars after this change: `DYNAMODB_TABLE`, `S3_BUCKET` (single bucket; `S3_ANSWER_IMAGE_BUCKET` retired).
- Old two-bucket data must keep working: S3 deletes parse bucket+key from the stored `image_url`.
- Per project CLAUDE.md: do not run `go build` after changes; verify with `go vet ./...` and `go test ./...`.
- No commits unless the user asks.

## File Structure

```
cmd/lambda/main.go           # package main; lambda.Start(httpadapter)
cmd/server/main.go           # local http server on :8080
cmd/backfill/main.go         # one-time entity_type backfill for existing items
internal/app/app.go          # dependency wiring -> http.Handler
internal/config/config.go    # env var loading with validation
internal/httpapi/router.go   # routes -> generic handlers
internal/httpapi/handlers.go # generic Resource[T,C,U] CRUD handlers
internal/httpapi/presign.go  # presigned-url handler
internal/httpapi/respond.go  # JSON + error responses (CORS everywhere)
internal/httpapi/cors.go     # CORS middleware incl. OPTIONS
internal/models/db.go        # + EntityType consts/fields, Card unchanged fields
internal/models/request.go   # + Card requests, validate tags on updates
internal/persistence/store.go      # generic DynamoDB ops (Get/Put/Update/Delete/Query)
internal/persistence/repository.go # Repository[T,C,U] iface + DynamoRepository
internal/persistence/entities.go   # per-entity repo constructors (GSI names, field maps)
internal/service/cascade.go  # cascading deletes incl. S3 objects
internal/storage/images.go   # ImageStore iface
internal/storage/s3.go       # S3 impl: PresignUpload, Delete(imageURL)
internal/testutil/fakes.go   # generic fake repo + fake image store
template.yaml                # SAM: table+5 GSIs, bucket, REST API w/ API key
Makefile                     # sam build target (GOOS=linux GOARCH=arm64)
DELETED: cmd/lambda/api/, internal/controllers/, internal/constants/, internal/utils/,
         internal/persistence/*DAO.go
```

**DynamoDB GSIs (all keys are existing attributes — no key migration):**
- `entity_type-index` (PK `entity_type`) — list categories/tags
- `category_id-index` (PK `category_id`) — decks by category
- `deck_id-index` (PK `deck_id`) — cards by deck
- `card_id-index` (PK `card_id`) — question images AND answer sections by card; disambiguated by `entity_type` filter (only place a filter is needed → backfill required only for these two entity types)
- `card_answer_section_id-index` (PK `card_answer_section_id`) — section images

### Task 1: Dependency

- [ ] `go get github.com/awslabs/aws-lambda-go-api-proxy` && `go mod tidy`
- Verify: `go vet ./...` clean.

### Task 2: Models

- [ ] Add `EntityType*` string consts for all 7 entities; add `EntityType` field to Deck, Card, CardAnswerSection, CardQuestionImage, CardAnswerSectionImage (`dynamodbav:"entity_type"`).
- [ ] Add `CreateCardRequest{DeckId required, Question required, TagIds}` and `UpdateCardRequest{Question required, TagIds, PreviouslyCorrect, LastAccessedDateTime}`.
- [ ] Add `validate:"required"` to Name (category/tag/deck updates), SequenceNumber (section/image updates), ImageURL (image updates).

### Task 3: Config

- [ ] `config.Load() (Config, error)` reading `DYNAMODB_TABLE`, `S3_BUCKET`; error naming the missing var. Test with `t.Setenv`.

### Task 4: Generic persistence

- Produces: `Store{DB *dynamodb.Client; Table string}`; funcs `Get[T]`, `Put`, `Update[T](id, attrs map[string]any)` (condition `attribute_exists(id)`, ALL_NEW, nil on cond-fail), `Delete[T]` (ALL_OLD, nil if absent), `Query[T](index, keyAttr, keyValue, entityTypeFilter)` (paginated).
- Produces: `Repository[T,C,U]` interface {List(parentID), Get, Create, Update, Delete}; `EntityConfig[T,C,U]{EntityType, ListIndex, ListKey, FilterByEntityType bool, New func(C) T, UpdateAttrs func(U) map[string]any}`; `NewDynamoRepository`.
- Produces (entities.go): `NewCategoryRepository(s)`, `NewDeckRepository`, `NewTagRepository`, `NewCardRepository`, `NewCardAnswerSectionRepository`, `NewCardQuestionImageRepository`, `NewCardAnswerSectionImageRepository`. Timestamps RFC3339 UTC on create; `updated_date_time` refreshed in UpdateAttrs where the model has it.
- [ ] Tests: entity `New` funcs set id/entity_type/timestamps; `UpdateAttrs` maps contain expected keys/values.

### Task 5: Storage

- Produces: `ImageStore{PresignUpload(ctx, prefix, fileName, contentType) (*PresignResult, error); Delete(ctx, imageURL) error}`; `PresignResult{UploadURL, ImageURL}`; `NewS3ImageStore(client *s3.Client, bucket string)`.
- Key layout: `<prefix>/<uuid><ext>`; prefixes `question-images`, `answer-images`. Presign 15 min, signs ContentType.
- `Delete` parses bucket from URL host (`x.s3.amazonaws.com` and `x.s3.<region>.amazonaws.com`) → old buckets still deletable.
- [ ] Tests: URL parse happy paths + error; key building.

### Task 6: Cascade service

- Produces: `service.Cascade` holding all 7 repos + ImageStore; `DeleteCategory/DeleteDeck/DeleteCard/DeleteSection/DeleteQuestionImage/DeleteSectionImage`, children deleted before parent; S3 object deletes are best-effort (log, don't fail the request) AFTER the record delete.
- [ ] Tests with fakes: category→deck→card→{question images, sections→section images} all deleted; ImageStore.Delete called per image; child failure aborts before parent delete.

### Task 7: httpapi

- Produces: `NewRouter(Deps) http.Handler`; `Deps{7 repos, Images ImageStore, Cascade *service.Cascade}`.
- Generic `Resource[T,C,U]{Repo, ListParam, DeleteFn}` with List/Get/Create/Update/Delete handlers. Semantics: 422 bad JSON, 400 missing param/validation, 404 nil results, 201 create, 200 otherwise. Lists return `[]` not `null`.
- CORS middleware on every response (`*`, `Content-Type,X-Api-Key`, `GET,POST,PUT,DELETE,OPTIONS`), OPTIONS → 204. This fixes errors-without-CORS.
- Presign handler: `fileName` required; `contentType` required, must start `image/`; `imageType=answer` → answer prefix.
- [ ] Tests via httptest against the real router with fakes: CRUD statuses, CORS header on 400/404, OPTIONS, update-validation 400, image delete calls ImageStore, presign validation.

### Task 8: Entry points + wiring

- Produces: `app.NewHandler(ctx) (http.Handler, error)` wiring config→clients→store→repos→cascade→router.
- `cmd/lambda/main.go`: **`package main`**, `lambda.Start(httpadapter.New(h).ProxyWithContext)`.
- `cmd/server/main.go`: `-addr :8080` flag, ListenAndServe.
- `cmd/backfill/main.go`: scan table, infer entity_type for legacy items (category_id→deck, deck_id→card, card_id+image_url→card_question_image, card_id→card_answer_section, card_answer_section_id→card_answer_section_image), `-dry-run` default true.

### Task 9: Remove old layers

- [ ] Delete `cmd/lambda/api/`, `internal/controllers/`, `internal/constants/`, `internal/utils/`, `internal/persistence/*DAO.go`.

### Task 10: SAM template + Makefile

- [ ] `template.yaml`: DynamoDB table + 5 GSIs (PAY_PER_REQUEST), S3 bucket (CORS for browser PUT, public-read policy w/ CloudFront note), Lambda (provided.al2023/arm64, env vars, DynamoDBCrudPolicy + S3CrudPolicy), REST API with `ApiKeyRequired` + usage plan, CORS preflight without key. `Makefile` `build-FlashcardFunction` for `sam build`.

### Task 11: Docs

- [ ] README: new architecture, routes (+card), env vars, local run, sam deploy, **Migration** section (add GSIs to existing tables via CLI, run backfill, bucket consolidation + IAM note for old buckets, API key/breaking presign contentType).
- [ ] `.claude/CLAUDE.md`: update architecture description; keep the no-go-build rule.

### Task 12: Verification

- [ ] `go vet ./...` clean; `go test ./...` all pass; confirm `go build -o <scratch>/bootstrap ./cmd/lambda/` — wait, per CLAUDE.md no `go build` unless asked; rely on `go vet`/`go test` compilation. Report results honestly.

## Self-Review

- Spec coverage: every review finding maps to a task (package main→8, CORS→7, delete ordering→5/6, cascades→6, update validation→2/7, auth→10, Scan→Query→4, DI/http.Handler→7/8, generics→4, single bucket→5, IaC→10, tests→3-7, Card CRUD→2/4/7, README/CLAUDE.md→11).
- Type consistency: `Repository[T,C,U]` shape shared by persistence/testutil/httpapi/service. `ImageStore` shared by storage/service/httpapi.
