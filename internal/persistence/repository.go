package persistence

import "context"

// Repository is the persistence interface handlers and services depend on.
// T is the entity, C the create request, U the update request. Get, Update
// and Delete return nil (no error) when the item does not exist.
type Repository[T any, C any, U any] interface {
	// List returns all entities whose parent key equals parentID. For
	// top-level entities (category, tag) parentID is ignored.
	List(ctx context.Context, parentID string) ([]T, error)
	Get(ctx context.Context, id string) (*T, error)
	Create(ctx context.Context, req C) (*T, error)
	Update(ctx context.Context, id string, req U) (*T, error)
	Delete(ctx context.Context, id string) (*T, error)
}

// EntityConfig carries the per-entity pieces the generic DynamoDB
// repository can't know: which GSI serves List, how to build a new entity
// from a create request, and which attributes an update sets.
type EntityConfig[T any, C any, U any] struct {
	EntityType string
	// ListIndex is the GSI queried by List; ListKey is its partition key
	// attribute. When ListKey is "entity_type" the index is keyed on the
	// entity type itself and List ignores parentID.
	ListIndex string
	ListKey   string
	// FilterByEntityType adds an entity_type filter to List queries; only
	// needed when two entity types share the same index key attribute.
	FilterByEntityType bool
	New                func(C) T
	UpdateAttrs        func(U) map[string]any
}

type DynamoRepository[T any, C any, U any] struct {
	store *Store
	cfg   EntityConfig[T, C, U]
}

func NewDynamoRepository[T any, C any, U any](store *Store, cfg EntityConfig[T, C, U]) *DynamoRepository[T, C, U] {
	return &DynamoRepository[T, C, U]{store: store, cfg: cfg}
}

func (r *DynamoRepository[T, C, U]) List(ctx context.Context, parentID string) ([]T, error) {
	if r.cfg.ListKey == "entity_type" {
		return QueryIndex[T](ctx, r.store, r.cfg.ListIndex, "entity_type", r.cfg.EntityType, "")
	}
	filter := ""
	if r.cfg.FilterByEntityType {
		filter = r.cfg.EntityType
	}
	return QueryIndex[T](ctx, r.store, r.cfg.ListIndex, r.cfg.ListKey, parentID, filter)
}

func (r *DynamoRepository[T, C, U]) Get(ctx context.Context, id string) (*T, error) {
	return GetItem[T](ctx, r.store, id)
}

func (r *DynamoRepository[T, C, U]) Create(ctx context.Context, req C) (*T, error) {
	entity := r.cfg.New(req)
	if err := PutItem(ctx, r.store, entity); err != nil {
		return nil, err
	}
	return &entity, nil
}

func (r *DynamoRepository[T, C, U]) Update(ctx context.Context, id string, req U) (*T, error) {
	return UpdateItem[T](ctx, r.store, id, r.cfg.UpdateAttrs(req))
}

func (r *DynamoRepository[T, C, U]) Delete(ctx context.Context, id string) (*T, error) {
	return DeleteItem[T](ctx, r.store, id)
}
