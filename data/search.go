package data

import (
	"context"
	"time"

	"github.com/pitabwire/frame/workerpool"
)

// defaultBatchSize limits pagination batches to keep memory usage predictable.
const defaultBatchSize = 50

// SearchQuery captures the minimal information a repository needs to translate
// a high-level search request into storage-specific operations. The query is
// constructed via functional options and is treated as immutable after
// creation so that the worker-pool driven search pipeline can safely share it
// between goroutines.
//
// Fields:
//   - `Query` holds the raw text or identifier provided by the caller. Repository
//     implementations are free to interpret it (for example, applying full-text
//     search) but should avoid mutating it.
//   - `FiltersOrByQuery` stores OR filters whose values indicate how the `Query`
//     should be interpolated into storage-level predicates (e.g. `LIKE ?`).
//   - `FiltersAndByValue` stores exact-match filters that are combined using AND
//     semantics.
//   - `TimePeriod` optionally restricts results to a date range on a specific
//     column.
//   - `Pagination` tracks the intended result window and streaming batch size.
type SearchQuery struct {
	Query string

	FiltersOrByQuery  map[string]string // We query with the value of query but use value as operator: {'id': ' = ?', 'name': ' LIKE ?', 'props': ' @@ plainto_tsquery(?)'}
	FiltersAndByValue map[string]any

	TimePeriod *TimePeriod
	Pagination *Paginator
}

// SearchOption mutates a `SearchQuery` during construction. Options are
// provided to `NewSearchQuery()` to override pagination defaults, attach
// filters, or scope execution to a time period.
type SearchOption func(*SearchQuery)

// NewSearchQuery builds a `SearchQuery` with sensible pagination defaults and
// applies the provided options. The paginator stores offsets in terms of page
// numbers; once options are applied we convert the supplied offset into the
// absolute row offset expected by downstream code.
func NewSearchQuery(query string, opts ...SearchOption) *SearchQuery {
	sq := SearchQuery{
		Query: query,
		Pagination: &Paginator{
			Offset:    0,
			Limit:     defaultBatchSize,
			BatchSize: defaultBatchSize,
		},
	}

	for _, opt := range opts {
		opt(&sq)
	}

	if sq.Pagination.Limit <= 0 {
		sq.Pagination.Limit = defaultBatchSize
	}

	if sq.Pagination.Limit <= defaultBatchSize {
		sq.Pagination.BatchSize = sq.Pagination.Limit
	}

	if sq.Pagination.BatchSize > defaultBatchSize {
		sq.Pagination.BatchSize = defaultBatchSize
	}

	sq.Pagination.Offset *= sq.Pagination.Limit

	return &sq
}

// Paginator tracks the current pagination state for a query. It keeps the
// desired `Limit` alongside the currently loaded `Offset` and the `BatchSize`
// used when streaming batched results through the worker pool.
type Paginator struct {
	Offset int
	Limit  int

	BatchSize int
}

// TimePeriod is an inclusive date range constraint that can be applied to a search
// query. The repository-layer decides which column is filtered via `Field`.
type TimePeriod struct {
	Field     string
	StartDate *time.Time
	StopDate  *time.Time
}

// CanLoad reports whether the paginator is still within the requested limit.
func (p *Paginator) CanLoad() bool {
	return p.Offset < p.Limit
}

// SetBatchSize overrides the streaming batch size. It is primarily used by
// tests to exercise edge cases.
func (p *Paginator) SetBatchSize(batchSize int) {
	p.BatchSize = batchSize
}

// Stop updates the offset after a batch is processed and indicates whether the
// search should stop because fewer rows than requested were returned.
func (p *Paginator) Stop(loadedCount int) bool {
	p.Offset += loadedCount
	if p.Offset+p.BatchSize > p.Limit {
		p.BatchSize = p.Limit - p.Offset
	}

	return loadedCount < p.BatchSize
}

// StableSearch executes the provided `searchFunc` until the paginator reports
// that all results have been consumed, streaming batches through the
// workerpool. It is safe to call from multiple goroutines; the provided
// `workerpool.Manager` coordinates execution and buffering of results.
func StableSearch[T any](
	ctx context.Context, workMan workerpool.Manager,
	query *SearchQuery, searchFunc func(ctx context.Context, query *SearchQuery) ([]T, error),
) (workerpool.JobResultPipe[[]T], error) {
	job := workerpool.NewJob(func(ctx context.Context, jobResult workerpool.JobResultPipe[[]T]) error {
		paginator := query.Pagination
		for paginator.CanLoad() {
			resultList, err := searchFunc(ctx, query)
			if err != nil {
				return jobResult.WriteError(ctx, err)
			}

			err = jobResult.WriteResult(ctx, resultList)
			if err != nil {
				return err
			}

			if paginator.Stop(len(resultList)) {
				break
			}
		}
		return nil
	})

	err := workerpool.SubmitJob(ctx, workMan, job)
	if err != nil {
		return nil, err
	}

	return job, nil
}

// WithSearchOffset configures the paginator to start from the provided page
// number. The actual row offset is derived inside `NewSearchQuery()`.
func WithSearchOffset(offset int) SearchOption {
	return func(query *SearchQuery) {
		query.Pagination.Offset = offset
	}
}

// WithSearchLimit sets the total number of rows the paginator should attempt to
// load before terminating.
func WithSearchLimit(limit int) SearchOption {
	return func(query *SearchQuery) {
		query.Pagination.Limit = limit
	}
}

// WithSearchBatchSize allows callers to request a specific streaming batch
// size. Values larger than `defaultBatchSize` are capped when the query is
// constructed.
func WithSearchBatchSize(batchSize int) SearchOption {
	return func(query *SearchQuery) {
		query.Pagination.BatchSize = batchSize
	}
}

// WithSearchFiltersOrByQuery supplies OR filters whose values are map entries
// describing the operator to use for the textual query, e.g. `LIKE ?`.
func WithSearchFiltersOrByQuery(filters map[string]string) SearchOption {
	return func(query *SearchQuery) {
		query.FiltersOrByQuery = filters
	}
}

// WithSearchFiltersAndByValue supplies AND filters whose values are compared
// for equality in repository implementations.
func WithSearchFiltersAndByValue(filters map[string]any) SearchOption {
	return func(query *SearchQuery) {
		query.FiltersAndByValue = filters
	}
}

// WithSearchByTimePeriod constrains the search to the provided date range.
func WithSearchByTimePeriod(period *TimePeriod) SearchOption {
	return func(query *SearchQuery) {
		query.TimePeriod = period
	}
}
