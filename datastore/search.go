package datastore

import (
	"context"

	"github.com/pitabwire/frame"
)

const defaultBatchSize = 50

type SearchQuery struct {
	ProfileID string
	Query     string
	Fields    map[string]any

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
	query *SearchQuery, searchFunc func(ctx context.Context, query *SearchQuery) ([]*T, error),
) (frame.JobResultPipe[[]*T], error) {
	job := frame.NewJob(func(ctx context.Context, jobResult frame.JobResultPipe[[]*T]) error {
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

	err := frame.SubmitJob(ctx, svc, job)
	if err != nil {
		return nil, err
	}

	return job, nil
}
