package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flashcard_lambda/internal/models"
	"flashcard_lambda/internal/persistence"
	"flashcard_lambda/internal/service"
	"flashcard_lambda/internal/storage"
	"flashcard_lambda/internal/testutil"
)

type budgetFunc func(context.Context, int64) error

func (f budgetFunc) ConsumeUpload(ctx context.Context, size int64) error { return f(ctx, size) }

func imageRouter(t *testing.T, objects *testutil.FakeImageStore, budget UploadBudget, create func(context.Context, models.CreateCardQuestionImageRequest) (*models.CardQuestionImage, error)) http.Handler {
	t.Helper()
	cards := &testutil.FakeRepo[models.Card, models.CreateCardRequest, models.UpdateCardRequest]{GetFn: func(_ context.Context, id string) (*models.Card, error) {
		if id != "card-1" {
			return nil, nil
		}
		return &models.Card{Id: id, EntityType: models.EntityTypeCard}, nil
	}}
	images := &testutil.FakeRepo[models.CardQuestionImage, models.CreateCardQuestionImageRequest, models.UpdateCardQuestionImageRequest]{CreateFn: create}
	return NewRouter(Deps{Cards: cards, QuestionImages: images, Images: objects, UploadBudget: budget, Authenticate: allowTestRequests, Cascade: &service.Cascade{}, AllowedOrigin: "http://localhost:5173"})
}
func uploadRequest(router http.Handler, body []byte, contentType, parent string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/card-question-image?cardId="+parent+"&sequenceNumber=1", bytes.NewReader(body))
	r.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, r)
	return rec
}
func smallPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestUploadValidatesBytesAndCreatesOnlyServerBoundImage(t *testing.T) {
	var savedKey string
	var savedBody []byte
	var created models.CreateCardQuestionImageRequest
	var charged int64
	objects := &testutil.FakeImageStore{PutFn: func(_ context.Context, key string, data []byte, mime string) error {
		savedKey = key
		savedBody = data
		if mime != "image/png" {
			t.Fatal(mime)
		}
		return nil
	}}
	router := imageRouter(t, objects, budgetFunc(func(_ context.Context, n int64) error { charged = n; return nil }), func(_ context.Context, req models.CreateCardQuestionImageRequest) (*models.CardQuestionImage, error) {
		created = req
		return &models.CardQuestionImage{Id: req.Id, StorageKey: req.StorageKey, CardId: req.CardId, SequenceNumber: req.SequenceNumber}, nil
	})
	response := uploadRequest(router, append(smallPNG(t), []byte("<script>trailer</script>")...), "image/png", "card-1")
	if response.Code != 201 {
		t.Fatalf("%d %s", response.Code, response.Body)
	}
	if created.CardId != "card-1" || created.SequenceNumber != 1 || storage.ValidateImageKey(created.Id, savedKey) != nil || created.StorageKey != savedKey {
		t.Fatalf("wrong immutable binding %+v / %s", created, savedKey)
	}
	if bytes.Contains(savedBody, []byte("trailer")) || charged != int64(len(savedBody)) {
		t.Fatal("unvalidated bytes or incorrect quota reservation")
	}
	var got models.CardQuestionImage
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.ImageURL, savedKey) || got.Id != created.Id || strings.Contains(response.Body.String(), "storage_key") {
		t.Fatalf("wrong response %s", response.Body)
	}
}

func TestUploadRejectsUnsafeInputBeforeWriting(t *testing.T) {
	for _, tc := range []struct {
		name, mime, parent string
		body               []byte
		status             int
	}{
		{"legacy JSON", "application/json", "card-1", []byte(`{"imageURL":"https://other.s3.amazonaws.com/a"}`), 415},
		{"SVG", "image/svg+xml", "card-1", []byte("<svg/>"), 415},
		{"disguised HTML", "image/png", "card-1", []byte("<html>bad</html>"), 422},
		{"missing parent", "image/png", "missing", smallPNG(t), 404},
		{"too large", "image/png", "card-1", make([]byte, storage.MaxUploadBytes+1), 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			objects := &testutil.FakeImageStore{PutFn: func(context.Context, string, []byte, string) error { t.Fatal("unsafe S3 write"); return nil }}
			router := imageRouter(t, objects, budgetFunc(func(context.Context, int64) error { t.Fatal("invalid input charged quota"); return nil }), func(context.Context, models.CreateCardQuestionImageRequest) (*models.CardQuestionImage, error) {
				t.Fatal("unsafe record write")
				return nil, nil
			})
			if got := uploadRequest(router, tc.body, tc.mime, tc.parent); got.Code != tc.status {
				t.Fatalf("%d %s", got.Code, got.Body)
			}
		})
	}
}

func TestUploadQuotaFailureDoesNotWriteObject(t *testing.T) {
	objects := &testutil.FakeImageStore{PutFn: func(context.Context, string, []byte, string) error { t.Fatal("quota bypass"); return nil }}
	router := imageRouter(t, objects, budgetFunc(func(context.Context, int64) error { return persistence.ErrUploadQuota }), nil)
	if got := uploadRequest(router, smallPNG(t), "image/png", "card-1"); got.Code != 429 {
		t.Fatalf("%d %s", got.Code, got.Body)
	}
}

func TestImageUpdateRejectsClientURL(t *testing.T) {
	f := newFixture()
	f.questionImages.UpdateFn = func(context.Context, string, models.UpdateCardQuestionImageRequest) (*models.CardQuestionImage, error) {
		t.Fatal("URL mutation reached repository")
		return nil, nil
	}
	got := f.do("PUT", "/card-question-image?id=image-1", `{"sequenceNumber":1,"imageURL":"https://other.s3.amazonaws.com/private.png"}`)
	if got.Code != 422 {
		t.Fatalf("%d %s", got.Code, got.Body)
	}
}

func TestImageReadDoesNotExposeLegacyURLOrMutateStoredRecord(t *testing.T) {
	f := newFixture()
	stored := models.CardQuestionImage{Id: "legacy", ImageURL: "https://legacy.s3.amazonaws.com/private.png"}
	f.questionImages.GetFn = func(context.Context, string) (*models.CardQuestionImage, error) { return &stored, nil }
	got := f.do("GET", "/card-question-image?id=legacy", "")
	if got.Code != 200 || strings.Contains(got.Body.String(), "legacy.s3") || stored.ImageURL == "" {
		t.Fatalf("unsafe response/storage mutation: %s %+v", got.Body, stored)
	}
}

func TestRouterFailsClosedWithoutAuthentication(t *testing.T) {
	router := NewRouter(Deps{Cascade: &service.Cascade{}, AllowedOrigin: "https://trusted.example"})
	r := httptest.NewRequest("GET", "/categories", nil)
	r.Header.Set("X-Api-Key", "old-exposed-key")
	r.Header.Set("Origin", "https://trusted.example")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, r)
	if rec.Code != 401 || rec.Header().Get("Access-Control-Allow-Origin") != "https://trusted.example" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %v", rec.Code, rec.Header())
	}
	r.Header.Set("Origin", "https://attacker.example")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, r)
	if rec.Code != 403 || rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("untrusted browser origin allowed")
	}
}
