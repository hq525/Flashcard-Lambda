// Package testutil provides in-memory fakes for the Repository and
// ImageStore interfaces, shared by httpapi and service tests.
package testutil

import (
	"context"
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

// FakeImageStore captures server-controlled object operations without AWS I/O.
type FakeImageStore struct {
	PutFn      func(context.Context, string, []byte, string) error
	ReadURLFn  func(context.Context, string) (string, error)
	DeleteErr  error
	DeletedURL []string // Stored managed keys (legacy name retained for test callers).
}

func (f *FakeImageStore) Put(ctx context.Context, key string, data []byte, contentType string) error {
	if f.PutFn != nil {
		return f.PutFn(ctx, key, data, contentType)
	}
	return nil
}
func (f *FakeImageStore) ReadURL(ctx context.Context, key string) (string, error) {
	if f.ReadURLFn != nil {
		return f.ReadURLFn(ctx, key)
	}
	return "https://test-bucket.s3.us-east-1.amazonaws.com/" + key + "?signed=test", nil
}
func (f *FakeImageStore) Delete(ctx context.Context, key string) error {
	f.DeletedURL = append(f.DeletedURL, key)
	return f.DeleteErr
}
