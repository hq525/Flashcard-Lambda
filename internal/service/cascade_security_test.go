package service

import (
	"context"
	"testing"

	"flashcard_lambda/internal/models"
	"flashcard_lambda/internal/testutil"
)

func TestImageDeletionRejectsUnboundKeyBeforeRemovingRecord(t *testing.T) {
	id := "e3d4a94b-f0e9-46af-a2c0-2c02850b539a"
	for _, key := range []string{"", "images/63315953-b5c4-4a0c-9459-787f61368daa.png"} {
		image := models.CardQuestionImage{Id: id, StorageKey: key, ImageURL: "https://legacy.s3.amazonaws.com/unrelated.png"}
		repo := &testutil.FakeRepo[models.CardQuestionImage, models.CreateCardQuestionImageRequest, models.UpdateCardQuestionImageRequest]{GetFn: func(context.Context, string) (*models.CardQuestionImage, error) { return &image, nil }, DeleteFn: func(context.Context, string) (*models.CardQuestionImage, error) { return &image, nil }}
		objects := &testutil.FakeImageStore{}
		cascade := &Cascade{QuestionImages: repo, Images: objects}
		if _, err := cascade.DeleteQuestionImage(context.Background(), id); err == nil {
			t.Fatal("unsafe image deletion accepted")
		}
		if len(repo.Deleted) != 0 || len(objects.DeletedURL) != 0 {
			t.Fatal("unsafe record or object deleted")
		}
	}
}
