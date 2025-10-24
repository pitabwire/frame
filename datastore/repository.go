package datastore

import (
	"context"
	"fmt"
	"time"

	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/datastore/pool"
	"github.com/pitabwire/frame/workerpool"
)

// BaseRepository provides generic CRUD operations for any model type.
// T is the model type (e.g., *models.Room).
type BaseRepository[T any] interface {
	Svc() pool.Pool
	GetByID(ctx context.Context, id string) (T, error)
	GetLastestBy(ctx context.Context, properties map[string]any) (T, error)
	GetAllBy(ctx context.Context, properties map[string]any, offset, limit int) ([]T, error)
	Search(ctx context.Context, query *data.SearchQuery) (workerpool.JobResultPipe[[]T], error)
	Count(ctx context.Context) (int64, error)
	Save(ctx context.Context, entity T) error
	Delete(ctx context.Context, id string) error
}

// baseRepository is the concrete implementation of BaseRepository.
type baseRepository[T data.BaseModelI] struct {
	dbPool  pool.Pool
	workMan workerpool.Manager
	// modelFactory creates a new instance of T for queries
	modelFactory func() T
}

// NewBaseRepository creates a new base repository instance.
// modelFactory should return a pointer to a new model instance (e.g., func() *models.Room { return &models.Room{} }).
func NewBaseRepository[T data.BaseModelI](
	dbPool pool.Pool,
	workMan workerpool.Manager,
	modelFactory func() T,
) BaseRepository[T] {
	return &baseRepository[T]{
		dbPool:       dbPool,
		workMan:      workMan,
		modelFactory: modelFactory,
	}
}

func (br *baseRepository[T]) Svc() pool.Pool {
	return br.dbPool
}

// GetByID retrieves an entity by its ID.
func (br *baseRepository[T]) GetByID(ctx context.Context, id string) (T, error) {
	entity := br.modelFactory()
	err := br.Svc().DB(ctx, true).First(entity, "id = ?", id).Error
	return entity, err
}

// Save creates or updates an entity.
func (br *baseRepository[T]) Save(ctx context.Context, entity T) error {
	if entity.GetVersion() <= 0 {
		return br.Svc().DB(ctx, false).Create(entity).Error
	}

	return br.Svc().DB(ctx, false).Save(entity).Error
}

// Delete soft deletes an entity by its ID.
func (br *baseRepository[T]) Delete(ctx context.Context, id string) error {
	entity, err := br.GetByID(ctx, id)
	if err != nil {
		return err
	}
	return br.Svc().DB(ctx, false).Delete(entity).Error
}

// Count returns the total number of entities.
func (br *baseRepository[T]) Count(ctx context.Context) (int64, error) {
	var count int64
	entity := br.modelFactory()
	err := br.Svc().DB(ctx, true).Model(entity).Count(&count).Error
	return count, err
}

func (br *baseRepository[T]) GetLastestBy(ctx context.Context, properties map[string]any) (T, error) {
	entity := br.modelFactory()
	query := br.Svc().DB(ctx, true)

	for key, value := range properties {
		query.Where(fmt.Sprintf("%s = ?", key), value)
	}

	err := query.Last(entity).Error
	return entity, err
}

func (br *baseRepository[T]) GetAllBy(ctx context.Context, properties map[string]any, offset, limit int) ([]T, error) {
	var entities []T

	query := br.Svc().DB(ctx, true).Offset(offset)

	if limit > 0 {
		query = query.Limit(limit)
	}

	for key, value := range properties {
		query.Where(fmt.Sprintf("%s = ?", key), value)
	}

	err := query.Find(entities).Error
	return entities, err
}

func (br *baseRepository[T]) Search(
	ctx context.Context,
	query *data.SearchQuery,
) (workerpool.JobResultPipe[[]T], error) {
	return data.StableSearch[T](
		ctx,
		br.workMan,
		query,
		func(ctx context.Context, query *data.SearchQuery) ([]T, error) {
			var entities []T

			paginator := query.Pagination

			db := br.Svc().DB(ctx, true).
				Limit(paginator.Limit).Offset(paginator.Offset)

			var startAt any
			var stopAt any
			for k, v := range query.Fields {
				if k == "start_date" {
					startAt = v
					continue
				}
				if k == "end_date" {
					stopAt = v
					continue
				}

				db = db.Where(fmt.Sprintf("%s = ? ", k), v)
			}

			if startAt != nil && stopAt != nil {
				startDate, ok1 := startAt.(*time.Time)
				endDate, ok2 := stopAt.(*time.Time)
				if ok1 && ok2 {
					db = db.Where(
						"created_at BETWEEN ? AND ? ",
						startDate.Format("2020-01-31T00:00:00Z"),
						endDate.Format("2020-01-31T00:00:00Z"),
					)
				}
			}

			if query.Query != "" {
				for searchField, oprt := range query.QueryFields {
					db = db.Where(fmt.Sprintf(" %s %s ", searchField, oprt), query.Query)
				}
			}

			err := db.Find(&entities).Error

			return entities, err
		},
	)
}
