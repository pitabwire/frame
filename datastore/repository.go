package datastore

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/datastore/pool"
	"github.com/pitabwire/frame/workerpool"
)

// BaseRepository provides generic CRUD operations for any model type.
// T is the model type (e.g., *models.Room).
type BaseRepository[T any] interface {
	Pool() pool.Pool
	GetByID(ctx context.Context, id string) (T, error)
	GetLastestBy(ctx context.Context, properties map[string]any) (T, error)
	GetAllBy(ctx context.Context, properties map[string]any, offset, limit int) ([]T, error)
	Search(ctx context.Context, query *data.SearchQuery) (workerpool.JobResultPipe[[]T], error)
	Count(ctx context.Context) (int64, error)
	CountBy(ctx context.Context, properties map[string]any) (int64, error)
	Create(ctx context.Context, entity T) error
	BatchSize() int
	BulkCreate(ctx context.Context, entities []T) error
	ImmutableFields() []string
	Update(ctx context.Context, entity T, affectedFields ...string) (int64, error)
	BulkUpdate(ctx context.Context, entityIDs []string, params map[string]any) (int64, error)
	Delete(ctx context.Context, id string) error
	DeleteBatch(ctx context.Context, ids []string) error
}

// baseRepository is the concrete implementation of BaseRepository.
type baseRepository[T data.BaseModelI] struct {
	dbPool  pool.Pool
	workMan workerpool.Manager
	// modelFactory creates a new instance of T for queries
	modelFactory func() T
	// tableName caches the table name to avoid repeated reflection
	tableName string

	batchSize       int
	immutableFields []string

	// allowedColumns whitelist for safe column access (set during initialization)
	allowedColumns map[string]bool
}

// NewBaseRepository creates a new base repository instance.
// modelFactory should return a pointer to a new model instance (e.g., func() *models.Room { return &models.Room{} }).
func NewBaseRepository[T data.BaseModelI](
	ctx context.Context,
	dbPool pool.Pool,
	workMan workerpool.Manager,
	modelFactory func() T,
) BaseRepository[T] {
	repo := &baseRepository[T]{
		dbPool:          dbPool,
		workMan:         workMan,
		modelFactory:    modelFactory,
		batchSize:       751, //nolint:mnd // default batch size
		immutableFields: []string{"id", "created_at", "tenant_id", "partition_id"},
		allowedColumns:  make(map[string]bool),
	}

	db := dbPool.DB(ctx, true)

	if db.CreateBatchSize > 0 {
		repo.batchSize = db.CreateBatchSize
	}

	// Initialize table name and allowed columns from model
	model := modelFactory()
	stmt := &gorm.Statement{DB: db}
	_ = stmt.Parse(model)
	repo.tableName = stmt.Schema.Table

	// Build allowed columns whitelist from schema
	for _, field := range stmt.Schema.Fields {
		repo.allowedColumns[field.DBName] = true
	}

	return repo
}

func (br *baseRepository[T]) Pool() pool.Pool {
	return br.dbPool
}

func (br *baseRepository[T]) ImmutableFields() []string {
	return br.immutableFields
}

// validateColumn checks if a column name is safe to use in queries.
func (br *baseRepository[T]) validateColumn(column string) error {
	if !br.allowedColumns[column] {
		return fmt.Errorf("invalid column name: %s", column)
	}
	return nil
}

// GetByID retrieves an entity by its ID.
func (br *baseRepository[T]) GetByID(ctx context.Context, id string) (T, error) {
	entity := br.modelFactory()
	// Use indexed lookup with prepared statement
	err := br.Pool().DB(ctx, true).Where("id = ?", id).First(entity).Error
	return entity, err
}

// Create creates a new entity in the database.
// It is intended for new entities and will return an error if the entity's version is greater than 0.
func (br *baseRepository[T]) Create(ctx context.Context, entity T) error {
	// Prevent updating existing entities with Create.
	if entity.GetVersion() > 0 {
		return errors.New("entity version is more than 0, consider using Update instead of Create")
	}

	// Use GORM's Create for new entities, which is more direct than Save.
	return br.Pool().DB(ctx, false).Create(entity).Error
}

func (br *baseRepository[T]) BatchSize() int {
	return br.batchSize
}

// BulkCreate inserts multiple entities efficiently in a single transaction.
func (br *baseRepository[T]) BulkCreate(ctx context.Context, entities []T) error {
	if len(entities) == 0 {
		return nil
	}

	// CreateInBatches uses GORM's batch insert which is more efficient
	// The batch size is configured in pool options (InsertBatchSize)
	return br.Pool().DB(ctx, false).CreateInBatches(entities, br.BatchSize()).Error
}

// validateAffectedColumns checks if all columns are valid and allowed.
func (br *baseRepository[T]) validateAffectedColumns(affectedColumns []string) error {
	for _, col := range affectedColumns {
		if err := br.validateColumn(col); err != nil {
			return err
		}
	}
	return nil
}

// Update updates an entity with optimistic locking.
// affectedFields specifies which columns to update. If empty, all non-zero fields are updated.
// Returns the number of rows affected.
func (br *baseRepository[T]) Update(ctx context.Context, entity T, affectedFields ...string) (int64, error) {
	// Validate entity has an ID
	if entity.GetID() == "" {
		return 0, errors.New("entity ID is required")
	}

	// Validate affected columns if provided
	if len(affectedFields) > 0 {
		if err := br.validateAffectedColumns(affectedFields); err != nil {
			return 0, err
		}
	}

	currentVersion := entity.GetVersion()

	// Build update query with optimistic locking (efficient single query)
	query := br.Pool().DB(ctx, false).
		Model(entity).
		Where("id = ? AND version = ?", entity.GetID(), currentVersion)

	// Apply column selection if specified
	if len(affectedFields) > 0 {
		query = query.Select(affectedFields)
	} else {
		// Only omit immutable fields when updating all fields
		query = query.Omit(br.ImmutableFields()...)
	}

	// Execute update - single database round trip
	result := query.Updates(entity)

	return result.RowsAffected, result.Error
}

// BulkUpdate efficiently updates multiple entities by their IDs with the same values.
// This performs a single UPDATE query with WHERE id IN (...) for maximum efficiency.
// Returns the number of rows affected.
func (br *baseRepository[T]) BulkUpdate(ctx context.Context, entityIDs []string, params map[string]any) (int64, error) {
	if len(entityIDs) == 0 {
		return 0, nil
	}

	if len(params) == 0 {
		return 0, errors.New("no parameters provided for update")
	}

	// Validate all column names in params
	for col := range params {
		if err := br.validateColumn(col); err != nil {
			return 0, err
		}
		for _, immutableField := range br.ImmutableFields() {
			if col == immutableField {
				return 0, fmt.Errorf("cannot bulk update immutable field: %s", col)
			}
		}
	}

	// Execute efficient bulk update - single query for all IDs
	// Use Table() instead of Model() to avoid GORM issues with empty entities
	result := br.Pool().DB(ctx, false).
		Table(br.tableName).
		Where("id IN ?", entityIDs).
		Updates(params)

	return result.RowsAffected, result.Error
}

// Delete soft deletes an entity by its ID without fetching it first.
func (br *baseRepository[T]) Delete(ctx context.Context, id string) error {
	// Direct delete without SELECT - much more efficient
	entity := br.modelFactory()
	return br.Pool().DB(ctx, false).Where("id = ?", id).Delete(entity).Error
}

// DeleteBatch soft deletes multiple entities by their IDs in a single query.
func (br *baseRepository[T]) DeleteBatch(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	entity := br.modelFactory()
	return br.Pool().DB(ctx, false).Where("id IN ?", ids).Delete(entity).Error
}

// Count returns the total number of entities.
func (br *baseRepository[T]) Count(ctx context.Context) (int64, error) {
	var count int64
	// Use table name directly instead of creating model instance
	err := br.Pool().DB(ctx, true).Table(br.tableName).Count(&count).Error
	return count, err
}

// CountBy returns the count of entities matching the given properties.
func (br *baseRepository[T]) CountBy(ctx context.Context, properties map[string]any) (int64, error) {
	var count int64
	query := br.Pool().DB(ctx, true).Table(br.tableName)

	// Apply filters with validation
	for key, value := range properties {
		if err := br.validateColumn(key); err != nil {
			return 0, err
		}
		query = query.Where(key+" = ?", value)
	}

	err := query.Count(&count).Error
	return count, err
}

// GetLastestBy retrieves the most recent entity matching the given properties.
func (br *baseRepository[T]) GetLastestBy(ctx context.Context, properties map[string]any) (T, error) {
	entity := br.modelFactory()
	query := br.Pool().DB(ctx, true)

	// Apply filters with validation
	for key, value := range properties {
		if err := br.validateColumn(key); err != nil {
			return entity, err
		}
		query = query.Where(key+" = ?", value)
	}

	// Order by created_at DESC for "latest"
	err := query.Order("created_at DESC").First(entity).Error
	return entity, err
}

// GetAllBy retrieves entities matching the given properties with pagination.
func (br *baseRepository[T]) GetAllBy(ctx context.Context, properties map[string]any, offset, limit int) ([]T, error) {
	var entities []T

	query := br.Pool().DB(ctx, true).Offset(offset)

	if limit > 0 {
		query = query.Limit(limit)
	}

	// Apply filters with validation
	for key, value := range properties {
		if err := br.validateColumn(key); err != nil {
			return nil, err
		}
		query = query.Where(key+" = ?", value)
	}

	// Fixed: Pass pointer to slice
	err := query.Find(&entities).Error
	return entities, err
}

// Search performs a complex search with pagination and filtering.
//
//nolint:gocognit // complexity is inherent to comprehensive search logic
func (br *baseRepository[T]) Search(
	ctx context.Context,
	query *data.SearchQuery,
) (workerpool.JobResultPipe[[]T], error) {
	return data.StableSearch[T](
		ctx, br.workMan, query,
		func(ctx context.Context, query *data.SearchQuery) ([]T, error) {
			var entities []T

			paginator := query.Pagination

			db := br.Pool().DB(ctx, true).
				Limit(paginator.Limit).
				Offset(paginator.Offset)

			if query.TimePeriod != nil {
				// Apply date range filter if range was provided
				if err := br.validateColumn(query.TimePeriod.Field); err != nil {
					return nil, err
				}

				// Use RFC3339 format for proper timezone handling
				db = db.Where(
					fmt.Sprintf("%s BETWEEN ? AND ?", query.TimePeriod.Field),
					query.TimePeriod.StartDate.Format(time.RFC3339),
					query.TimePeriod.StopDate.Format(time.RFC3339))
			}

			if len(query.FiltersAndByValue) > 0 {
				// Apply field filters with validation
				for k, v := range query.FiltersAndByValue {
					// Validate column name before using
					if err := br.validateColumn(k); err != nil {
						return nil, err
					}

					kind := reflect.TypeOf(v).Kind()
					if kind == reflect.Slice || kind == reflect.Array {
						db = db.Where(k+" IN ?", v)
					} else {
						db = db.Where(k+" = ?", v)
					}
				}
			}

			// Apply text search across multiple fields
			// ---- OR Text Search ----
			if query.Query != "" && len(query.FiltersOrByQuery) > 0 {
				var orClauses []string
				var orValues []any

				for field, operator := range query.FiltersOrByQuery {
					if err := br.validateColumn(field); err != nil {
						return nil, err
					}

					op := strings.TrimSpace(strings.ToUpper(operator))

					queryValue := query.Query
					if strings.Contains(op, "LIKE") {
						queryValue = "%" + query.Query + "%"
					}

					orClauses = append(orClauses, fmt.Sprintf("%s %s", field, op))
					orValues = append(orValues, queryValue)
				}

				if len(orClauses) > 0 {
					// Example: (name ILIKE ? OR topic ILIKE ?)
					combined := "(" + strings.Join(orClauses, " OR ") + ")"
					db = db.Where(combined, orValues...)
				}
			}

			// Execute query with pointer to slice
			err := db.Find(&entities).Error

			return entities, err
		})
}
