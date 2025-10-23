package framedata

import (
	"context"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/workerpool"
)

const defaultBatchSize = 50

type SearchQuery struct {
	ProfileID   string
	Query       string
	QueryFields map[string]string // We query with the value of query but use value as operator: {'id': ' = ?', 'name': ' LIKE ?', 'props': ' @@ plainto_tsquery(?)'}
	Fields      map[string]any

	Pagination *Paginator
}

func NewSearchQuery(query string,
	fields map[string]any,
	resultPage, resultCount int,
) *SearchQuery {
	if resultCount == 0 {
		resultCount = defaultBatchSize
	}

	batchSize := resultCount
	if batchSize > defaultBatchSize {
		batchSize = defaultBatchSize
	}

	sq := &SearchQuery{
		Query:  query,
		Fields: fields,
		Pagination: &Paginator{
			Offset:    resultPage * resultCount,
			Limit:     resultCount,
			BatchSize: batchSize,
		},
	}

	return sq
}

type Paginator struct {
	Offset int
	Limit  int

	BatchSize int
}

func (p *Paginator) CanLoad() bool {
	return p.Offset < p.Limit
}

func (p *Paginator) SetBatchSize(batchSize int) {
	p.BatchSize = batchSize
}

func (p *Paginator) Stop(loadedCount int) bool {
	p.Offset += loadedCount
	if p.Offset+p.BatchSize > p.Limit {
		p.BatchSize = p.Limit - p.Offset
	}

	return loadedCount < p.BatchSize
}

func StableSearch[T any](
	ctx context.Context, svc *frame.Service,
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

	err := workerpool.SubmitJob(ctx, svc.WorkManager(), job)
	if err != nil {
		return nil, err
	}

	return job, nil
}
