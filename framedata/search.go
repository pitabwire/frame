package framedata

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

func StableSearch(
	ctx context.Context, svc *frame.Service,
	query *SearchQuery, searchFunc func(ctx context.Context, query *SearchQuery) ([]interface{}, error),
) (frame.JobResultPipe, error) {
	job := frame.NewJob("stable_search", nil)

	err := frame.SubmitJob(job)
	if err != nil {
		return nil, err
	}

	return job, nil
}
