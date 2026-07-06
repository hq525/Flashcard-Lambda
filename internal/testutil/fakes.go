// Package testutil provides in-memory fakes for the Repository and
// ImageStore interfaces, shared by httpapi and service tests.
package testutil

import (
	"context"

	"flashcard_lambda/internal/storage"
)

// FakeRepo implements persistence.Repository[T, C, U]. Tests set only the
// function fields they need; calling an unset operation panics, which
// surfaces unexpected calls immediately.
type FakeRepo[T any, C any, U any] struct {
	ListFn   func(ctx context.Context, parentID string) ([]T, error)
	GetFn    func(ctx context.Context, id string) (*T, error)
	CreateFn func(ctx context.Context, req C) (*T, error)
	UpdateFn func(ctx context.Context, id string, req U) (*T, error)
	DeleteFn func(ctx context.Context, id string) (*T, error)

	Deleted []string
}

func (f *FakeRepo[T, C, U]) List(ctx context.Context, parentID string) ([]T, error) {
	return f.ListFn(ctx, parentID)
}

func (f *FakeRepo[T, C, U]) Get(ctx context.Context, id string) (*T, error) {
	return f.GetFn(ctx, id)
}

func (f *FakeRepo[T, C, U]) Create(ctx context.Context, req C) (*T, error) {
	return f.CreateFn(ctx, req)
}

func (f *FakeRepo[T, C, U]) Update(ctx context.Context, id string, req U) (*T, error) {
	return f.UpdateFn(ctx, id, req)
}

func (f *FakeRepo[T, C, U]) Delete(ctx context.Context, id string) (*T, error) {
	f.Deleted = append(f.Deleted, id)
	return f.DeleteFn(ctx, id)
}

// FakeImageStore implements storage.ImageStore and records deleted URLs.
type FakeImageStore struct {
	PresignFn  func(ctx context.Context, prefix, fileName, contentType string) (*storage.PresignResult, error)
	DeleteErr  error
	DeletedURL []string
}

func (f *FakeImageStore) PresignUpload(ctx context.Context, prefix, fileName, contentType string) (*storage.PresignResult, error) {
	return f.PresignFn(ctx, prefix, fileName, contentType)
}

func (f *FakeImageStore) Delete(ctx context.Context, imageURL string) error {
	f.DeletedURL = append(f.DeletedURL, imageURL)
	return f.DeleteErr
}
