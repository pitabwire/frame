package datastore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/datastore/pool"
	"github.com/pitabwire/frame/workerpool"
	"gorm.io/gorm"
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
	CountBy(ctx context.Context, properties map[string]any) (int64, error)
	Save(ctx context.Context, entity T) error
	BatchInsert(ctx context.Context, entities []T) error
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
	// allowedColumns whitelist for safe column access (set during initialization)
	allowedColumns map[string]bool
}

// NewBaseRepository creates a new base repository instance.
// modelFactory should return a pointer to a new model instance (e.g., func() *models.Room { return &models.Room{} }).
func NewBaseRepository[T data.BaseModelI](
	dbPool pool.Pool,
	workMan workerpool.Manager,
	modelFactory func() T,
) BaseRepository[T] {
	repo := &baseRepository[T]{
		dbPool:         dbPool,
		workMan:        workMan,
		modelFactory:   modelFactory,
		allowedColumns: make(map[string]bool),
	}

	// Initialize table name and allowed columns from model
	model := modelFactory()
	stmt := &gorm.Statement{DB: dbPool.DB(context.Background(), true)}
	_ = stmt.Parse(model)
	repo.tableName = stmt.Schema.Table

	// Build allowed columns whitelist from schema
	for _, field := range stmt.Schema.Fields {
		repo.allowedColumns[field.DBName] = true
	}

	return repo
}

func (br *baseRepository[T]) Svc() pool.Pool {
	return br.dbPool
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
	err := br.Svc().DB(ctx, true).Where("id = ?", id).First(entity).Error
	return entity, err
}

// Save creates or updates an entity with optimistic locking.
// For new entities (version <= 0), it performs a CREATE operation.
// For existing entities, it performs an UPDATE with version check to prevent lost updates.
func (br *baseRepository[T]) Save(ctx context.Context, entity T) error {
	// Validate entity has an ID for updates
	if entity.GetVersion() > 0 && entity.GetID() == "" {
		return fmt.Errorf("entity ID is required for updates")
	}

	if entity.GetVersion() <= 0 {
		// Use Create for new entities (more efficient than Save)
		return br.Svc().DB(ctx, false).Create(entity).Error
	}

	// Update with optimistic locking to prevent lost updates
	// The version check ensures the entity hasn't been modified by another transaction
	currentVersion := entity.GetVersion()

	// GORM will increment version in BeforeUpdate hook
	// We check that exactly 1 row was updated with the expected version
	result := br.Svc().DB(ctx, false).
		Model(entity).
		Where("id = ? AND version = ?", entity.GetID(), currentVersion).
		Updates(entity)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("optimistic lock failed: entity (id=%s) was modified by another transaction (expected version: %d)", 
			entity.GetID(), currentVersion)
	}

	return nil
}

// BatchInsert inserts multiple entities efficiently in a single transaction.
func (br *baseRepository[T]) BatchInsert(ctx context.Context, entities []T) error {
	if len(entities) == 0 {
		return nil
	}

	// CreateInBatches uses GORM's batch insert which is more efficient
	// The batch size is configured in pool options (InsertBatchSize)
	return br.Svc().DB(ctx, false).CreateInBatches(entities, 0).Error
}

// Delete soft deletes an entity by its ID without fetching it first.
func (br *baseRepository[T]) Delete(ctx context.Context, id string) error {
	// Direct delete without SELECT - much more efficient
	entity := br.modelFactory()
	return br.Svc().DB(ctx, false).Where("id = ?", id).Delete(entity).Error
}

// DeleteBatch soft deletes multiple entities by their IDs in a single query.
func (br *baseRepository[T]) DeleteBatch(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	entity := br.modelFactory()
	return br.Svc().DB(ctx, false).Where("id IN ?", ids).Delete(entity).Error
}

// Count returns the total number of entities.
func (br *baseRepository[T]) Count(ctx context.Context) (int64, error) {
	var count int64
	// Use table name directly instead of creating model instance
	err := br.Svc().DB(ctx, true).Table(br.tableName).Count(&count).Error
	return count, err
}

// CountBy returns the count of entities matching the given properties.
func (br *baseRepository[T]) CountBy(ctx context.Context, properties map[string]any) (int64, error) {
	var count int64
	query := br.Svc().DB(ctx, true).Table(br.tableName)

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
	query := br.Svc().DB(ctx, true)

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

	query := br.Svc().DB(ctx, true).Offset(offset)

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
				Limit(paginator.Limit).
				Offset(paginator.Offset)

			// Process date range filters
			var startAt *time.Time
			var stopAt *time.Time

			// Apply field filters with validation
			for k, v := range query.Fields {
				if k == "start_date" {
					if t, ok := v.(*time.Time); ok {
						startAt = t
					}
					continue
				}
				if k == "end_date" {
					if t, ok := v.(*time.Time); ok {
						stopAt = t
					}
					continue
				}

				// Validate column name before using
				if err := br.validateColumn(k); err != nil {
					return nil, err
				}
				db = db.Where(k+" = ?", v)
			}

			// Apply date range filter if both dates provided
			if startAt != nil && stopAt != nil {
				// Fixed: Use actual dates, not hardcoded 2020
				// Use RFC3339 format for proper timezone handling
				db = db.Where(
					"created_at BETWEEN ? AND ?",
					startAt.Format(time.RFC3339),
					stopAt.Format(time.RFC3339),
				)
			}

			// Apply text search across multiple fields
			if query.Query != "" && len(query.QueryFields) > 0 {
				// Build OR conditions for search fields
				var conditions []string
				var args []interface{}

				for searchField, operator := range query.QueryFields {
					// Validate column name
					if err := br.validateColumn(searchField); err != nil {
						return nil, err
					}

					// Sanitize operator (whitelist allowed operators)
					operator = strings.TrimSpace(strings.ToUpper(operator))
					switch operator {
					case "LIKE", "ILIKE", "=", "!=", ">", "<", ">=", "<=":
						conditions = append(conditions, fmt.Sprintf("%s %s ?", searchField, operator))
						
						// Add wildcards for LIKE/ILIKE operators
						if operator == "LIKE" || operator == "ILIKE" {
							args = append(args, "%"+query.Query+"%")
						} else {
							args = append(args, query.Query)
						}
					default:
						return nil, fmt.Errorf("invalid operator: %s", operator)
					}
				}

				if len(conditions) > 0 {
					// Combine with OR
					db = db.Where(strings.Join(conditions, " OR "), args...)
				}
			}

			// Execute query with pointer to slice
			err := db.Find(&entities).Error

			return entities, err
		},
	)
}
