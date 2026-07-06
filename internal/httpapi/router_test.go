package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flashcard_lambda/internal/models"
	"flashcard_lambda/internal/service"
	"flashcard_lambda/internal/storage"
	"flashcard_lambda/internal/testutil"
)

type fixture struct {
	router         http.Handler
	categories     *testutil.FakeRepo[models.Category, models.CreateCategoryRequest, models.UpdateCategoryRequest]
	decks          *testutil.FakeRepo[models.Deck, models.CreateDeckRequest, models.UpdateDeckRequest]
	questionImages *testutil.FakeRepo[models.CardQuestionImage, models.CreateCardQuestionImageRequest, models.UpdateCardQuestionImageRequest]
	images         *testutil.FakeImageStore
}

func newFixture() *fixture {
	f := &fixture{
		categories:     &testutil.FakeRepo[models.Category, models.CreateCategoryRequest, models.UpdateCategoryRequest]{},
		decks:          &testutil.FakeRepo[models.Deck, models.CreateDeckRequest, models.UpdateDeckRequest]{},
		questionImages: &testutil.FakeRepo[models.CardQuestionImage, models.CreateCardQuestionImageRequest, models.UpdateCardQuestionImageRequest]{},
		images:         &testutil.FakeImageStore{},
	}
	tags := &testutil.FakeRepo[models.Tag, models.CreateTagRequest, models.UpdateTagRequest]{}
	cards := &testutil.FakeRepo[models.Card, models.CreateCardRequest, models.UpdateCardRequest]{}
	sections := &testutil.FakeRepo[models.CardAnswerSection, models.CreateCardAnswerSectionRequest, models.UpdateCardAnswerSectionRequest]{}
	sectionImages := &testutil.FakeRepo[models.CardAnswerSectionImage, models.CreateCardAnswerSectionImageRequest, models.UpdateCardAnswerSectionImageRequest]{}

	cascade := &service.Cascade{
		Categories:     f.categories,
		Decks:          f.decks,
		Cards:          cards,
		Sections:       sections,
		QuestionImages: f.questionImages,
		SectionImages:  sectionImages,
		Images:         f.images,
	}
	f.router = NewRouter(Deps{
		Categories:     f.categories,
		Decks:          f.decks,
		Tags:           tags,
		Cards:          cards,
		Sections:       sections,
		QuestionImages: f.questionImages,
		SectionImages:  sectionImages,
		Images:         f.images,
		Cascade:        cascade,
	})
	return f
}

func (f *fixture) do(method, target, body string) *httptest.ResponseRecorder {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func TestListCategories(t *testing.T) {
	f := newFixture()
	f.categories.ListFn = func(ctx context.Context, parentID string) ([]models.Category, error) {
		return []models.Category{{Id: "c1", Name: "Go"}}, nil
	}

	rec := f.do("GET", "/categories", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	var got []models.Category
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got) != 1 {
		t.Fatalf("body = %s (err %v)", rec.Body, err)
	}
}

func TestListReturnsEmptyArrayNotNull(t *testing.T) {
	f := newFixture()
	f.categories.ListFn = func(ctx context.Context, parentID string) ([]models.Category, error) {
		return nil, nil
	}

	rec := f.do("GET", "/categories", "")
	if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
		t.Errorf("empty list body = %q, want []", body)
	}
}

func TestListDecksRequiresCategoryId(t *testing.T) {
	f := newFixture()
	rec := f.do("GET", "/decks", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// The review's key CORS regression: error responses must carry CORS
// headers or browsers mask the real status.
func TestErrorResponsesCarryCORSHeaders(t *testing.T) {
	f := newFixture()
	f.categories.GetFn = func(ctx context.Context, id string) (*models.Category, error) {
		return nil, nil
	}

	for name, rec := range map[string]*httptest.ResponseRecorder{
		"400 missing param": f.do("GET", "/decks", ""),
		"404 not found":     f.do("GET", "/category?id=missing", ""),
		"404 bad route":     f.do("GET", "/nope", ""),
	} {
		if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Errorf("%s: missing Access-Control-Allow-Origin header", name)
		}
	}
}

func TestPreflight(t *testing.T) {
	f := newFixture()
	rec := f.do("OPTIONS", "/category", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	methods := rec.Header().Get("Access-Control-Allow-Methods")
	for _, m := range []string{"PUT", "DELETE"} {
		if !strings.Contains(methods, m) {
			t.Errorf("Allow-Methods %q missing %s", methods, m)
		}
	}
}

func TestGetCategoryNotFound(t *testing.T) {
	f := newFixture()
	f.categories.GetFn = func(ctx context.Context, id string) (*models.Category, error) {
		return nil, nil
	}
	if rec := f.do("GET", "/category?id=x", ""); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestCreateCategory(t *testing.T) {
	f := newFixture()
	f.categories.CreateFn = func(ctx context.Context, req models.CreateCategoryRequest) (*models.Category, error) {
		return &models.Category{Id: "new", Name: req.Name}, nil
	}

	rec := f.do("POST", "/category", `{"name":"Go"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body %s", rec.Code, rec.Body)
	}
}

func TestCreateCategoryBadJSON(t *testing.T) {
	f := newFixture()
	if rec := f.do("POST", "/category", `{not json`); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rec.Code)
	}
}

func TestCreateCategoryMissingName(t *testing.T) {
	f := newFixture()
	if rec := f.do("POST", "/category", `{"description":"no name"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// The review's update-validation regression: PUT {} must not blank fields.
func TestUpdateCategoryEmptyBodyRejected(t *testing.T) {
	f := newFixture()
	f.categories.UpdateFn = func(ctx context.Context, id string, req models.UpdateCategoryRequest) (*models.Category, error) {
		t.Fatal("Update should not be reached with an invalid body")
		return nil, nil
	}

	if rec := f.do("PUT", "/category?id=c1", `{}`); rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestDeleteCategoryCascadesFromRouter(t *testing.T) {
	f := newFixture()
	f.decks.ListFn = func(ctx context.Context, parentID string) ([]models.Deck, error) {
		return nil, nil
	}
	f.categories.DeleteFn = func(ctx context.Context, id string) (*models.Category, error) {
		return &models.Category{Id: id}, nil
	}

	rec := f.do("DELETE", "/category?id=c1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if len(f.categories.Deleted) != 1 || f.categories.Deleted[0] != "c1" {
		t.Errorf("deleted = %v, want [c1]", f.categories.Deleted)
	}
}

func TestDeleteQuestionImageDeletesS3Object(t *testing.T) {
	f := newFixture()
	f.questionImages.DeleteFn = func(ctx context.Context, id string) (*models.CardQuestionImage, error) {
		return &models.CardQuestionImage{Id: id, ImageURL: "https://b.s3.amazonaws.com/question-images/x.png"}, nil
	}

	rec := f.do("DELETE", "/card-question-image?id=qi1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if len(f.images.DeletedURL) != 1 {
		t.Errorf("S3 deletes = %v, want one", f.images.DeletedURL)
	}
}

func TestPresignedURL(t *testing.T) {
	f := newFixture()
	f.images.PresignFn = func(ctx context.Context, prefix, fileName, contentType string) (*storage.PresignResult, error) {
		if prefix != storage.AnswerImagePrefix {
			t.Errorf("prefix = %q, want %q", prefix, storage.AnswerImagePrefix)
		}
		return &storage.PresignResult{UploadURL: "https://signed", ImageURL: "https://img"}, nil
	}

	rec := f.do("GET", "/presigned-url?fileName=a.png&contentType=image/png&imageType=answer", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
}

func TestPresignedURLRequiresImageContentType(t *testing.T) {
	f := newFixture()
	for _, target := range []string{
		"/presigned-url?fileName=a.png",                               // missing contentType
		"/presigned-url?fileName=a.png&contentType=application/x-elf", // not an image
		"/presigned-url?contentType=image/png",                        // missing fileName
	} {
		if rec := f.do("GET", target, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", target, rec.Code)
		}
	}
}
