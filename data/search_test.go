package data_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/frametests/deps/testnats"
	"github.com/pitabwire/frame/frametests/deps/testpostgres"
	"github.com/pitabwire/frame/workerpool"
)

// SearchTestSuite extends FrameBaseTestSuite for comprehensive search testing.
type SearchTestSuite struct {
	frametests.FrameBaseTestSuite
}

// TestSearchSuite runs the search test suite.
func TestSearchSuite(t *testing.T) {
	suite.Run(t, &SearchTestSuite{
		FrameBaseTestSuite: frametests.FrameBaseTestSuite{
			InitResourceFunc: func(_ context.Context) []definition.TestResource {
				return []definition.TestResource{
					testpostgres.New(),
					testnats.New(),
				}
			},
		},
	})
}

// TestNewSearchQuery tests the NewSearchQuery function with various parameters.
func (s *SearchTestSuite) TestNewSearchQuery() {
	testCases := []struct {
		name          string
		query         string
		fields        map[string]any
		resultPage    int
		resultCount   int
		expectError   bool
		expectedQuery *data.SearchQuery
	}{
		{
			name:        "valid query with default count",
			query:       "test query",
			fields:      map[string]any{"field1": "value1"},
			resultPage:  0,
			resultCount: 0,
			expectError: false,
			expectedQuery: &data.SearchQuery{
				Query:  "test query",
				Fields: map[string]any{"field1": "value1"},
				Pagination: &data.Paginator{
					Offset:    0,
					Limit:     50, // defaultBatchSize
					BatchSize: 50,
				},
			},
		},
		{
			name:        "valid query with custom count",
			query:       "search term",
			fields:      map[string]any{"name": "John", "age": 30},
			resultPage:  1,
			resultCount: 25,
			expectError: false,
			expectedQuery: &data.SearchQuery{
				Query:  "search term",
				Fields: map[string]any{"name": "John", "age": 30},
				Pagination: &data.Paginator{
					Offset:    25,
					Limit:     25,
					BatchSize: 25,
				},
			},
		},
		{
			name:        "large result count capped to default batch size",
			query:       "large query",
			fields:      map[string]any{},
			resultPage:  0,
			resultCount: 100,
			expectError: false,
			expectedQuery: &data.SearchQuery{
				Query:  "large query",
				Fields: map[string]any{},
				Pagination: &data.Paginator{
					Offset:    0,
					Limit:     100,
					BatchSize: 50, // defaultBatchSize
				},
			},
		},
		{
			name:        "empty query with fields",
			query:       "",
			fields:      map[string]any{"status": "active"},
			resultPage:  2,
			resultCount: 10,
			expectError: false,
			expectedQuery: &data.SearchQuery{
				Query:  "",
				Fields: map[string]any{"status": "active"},
				Pagination: &data.Paginator{
					Offset:    20,
					Limit:     10,
					BatchSize: 10,
				},
			},
		},
		{
			name:        "nil fields map",
			query:       "test",
			fields:      nil,
			resultPage:  0,
			resultCount: 15,
			expectError: false,
			expectedQuery: &data.SearchQuery{
				Query:  "test",
				Fields: nil,
				Pagination: &data.Paginator{
					Offset:    0,
					Limit:     15,
					BatchSize: 15,
				},
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			result := data.NewSearchQuery(tc.query, tc.fields, tc.resultPage, tc.resultCount)

			s.NotNil(result)
			s.Equal(tc.expectedQuery.Query, result.Query)
			s.Equal(tc.expectedQuery.Fields, result.Fields)
			s.Equal(tc.expectedQuery.Pagination.Offset, result.Pagination.Offset)
			s.Equal(tc.expectedQuery.Pagination.Limit, result.Pagination.Limit)
			s.Equal(tc.expectedQuery.Pagination.BatchSize, result.Pagination.BatchSize)
		})
	}
}

// TestPaginatorCanLoad tests the CanLoad method with various offset/limit combinations.
func (s *SearchTestSuite) TestPaginatorCanLoad() {
	testCases := []struct {
		name     string
		offset   int
		limit    int
		expected bool
	}{
		{
			name:     "can load when offset less than limit",
			offset:   10,
			limit:    50,
			expected: true,
		},
		{
			name:     "cannot load when offset equals limit",
			offset:   25,
			limit:    25,
			expected: false,
		},
		{
			name:     "cannot load when offset greater than limit",
			offset:   100,
			limit:    50,
			expected: false,
		},
		{
			name:     "can load at start",
			offset:   0,
			limit:    10,
			expected: true,
		},
		{
			name:     "cannot load with zero limit",
			offset:   0,
			limit:    0,
			expected: false,
		},
		{
			name:     "edge case - offset one less than limit",
			offset:   49,
			limit:    50,
			expected: true,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			paginator := &data.Paginator{
				Offset: tc.offset,
				Limit:  tc.limit,
			}

			result := paginator.CanLoad()
			s.Equal(tc.expected, result)
		})
	}
}

// TestPaginatorStop tests the Stop method with different loaded counts and batch sizes.
func (s *SearchTestSuite) TestPaginatorStop() {
	testCases := []struct {
		name              string
		initialOffset     int
		initialLimit      int
		initialBatchSize  int
		loadedCount       int
		expectedStop      bool
		expectedOffset    int
		expectedBatchSize int
	}{
		{
			name:              "continue loading - full batch loaded",
			initialOffset:     0,
			initialLimit:      100,
			initialBatchSize:  25,
			loadedCount:       25,
			expectedStop:      false,
			expectedOffset:    25,
			expectedBatchSize: 25,
		},
		{
			name:              "stop loading - partial batch loaded",
			initialOffset:     0,
			initialLimit:      100,
			initialBatchSize:  25,
			loadedCount:       15,
			expectedStop:      true,
			expectedOffset:    15,
			expectedBatchSize: 25,
		},
		{
			name:              "adjust batch size near limit",
			initialOffset:     80,
			initialLimit:      100,
			initialBatchSize:  25,
			loadedCount:       20,
			expectedStop:      false,
			expectedOffset:    100,
			expectedBatchSize: 0,
		},
		{
			name:              "stop at exact limit",
			initialOffset:     75,
			initialLimit:      100,
			initialBatchSize:  25,
			loadedCount:       25,
			expectedStop:      false,
			expectedOffset:    100,
			expectedBatchSize: 0,
		},
		{
			name:              "no items loaded",
			initialOffset:     10,
			initialLimit:      50,
			initialBatchSize:  20,
			loadedCount:       0,
			expectedStop:      true,
			expectedOffset:    10,
			expectedBatchSize: 20,
		},
		{
			name:              "single item batch",
			initialOffset:     0,
			initialLimit:      10,
			initialBatchSize:  1,
			loadedCount:       1,
			expectedStop:      false,
			expectedOffset:    1,
			expectedBatchSize: 1,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			paginator := &data.Paginator{
				Offset:    tc.initialOffset,
				Limit:     tc.initialLimit,
				BatchSize: tc.initialBatchSize,
			}

			result := paginator.Stop(tc.loadedCount)

			s.Equal(tc.expectedStop, result)
			s.Equal(tc.expectedOffset, paginator.Offset)
			s.Equal(tc.expectedBatchSize, paginator.BatchSize)
		})
	}
}

// TestStableSearchWithDependencies tests StableSearch with real infrastructure dependencies.
func (s *SearchTestSuite) TestStableSearchWithDependencies() {
	depOptions := []*definition.DependancyOption{
		definition.NewDependancyOption("postgres_nats", "test", s.Resources()),
	}

	frametests.WithTestDependancies(s.T(), depOptions, func(t *testing.T, depOpt *definition.DependancyOption) {
		testCases := []struct {
			name          string
			query         *data.SearchQuery
			searchFunc    func(ctx context.Context, query *data.SearchQuery) ([]*TestItem, error)
			expectedItems int
			expectedError bool
		}{
			{
				name: "successful search with single batch",
				query: &data.SearchQuery{
					Query:  "test",
					Fields: map[string]any{"category": "books"},
					Pagination: &data.Paginator{
						Offset:    0,
						Limit:     10,
						BatchSize: 10,
					},
				},
				searchFunc: func(_ context.Context, _ *data.SearchQuery) ([]*TestItem, error) {
					return []*TestItem{
						{ID: "1", Name: "Item 1"},
						{ID: "2", Name: "Item 2"},
					}, nil
				},
				expectedItems: 2,
				expectedError: false,
			},
			{
				name: "successful search with multiple batches",
				query: &data.SearchQuery{
					Query:  "multi",
					Fields: map[string]any{},
					Pagination: &data.Paginator{
						Offset:    0,
						Limit:     25,
						BatchSize: 10,
					},
				},
				searchFunc:    createMultiBatchSearchFunc(),
				expectedItems: 25,
				expectedError: false,
			},
			{
				name: "search function returns error",
				query: &data.SearchQuery{
					Query:  "error",
					Fields: map[string]any{},
					Pagination: &data.Paginator{
						Offset:    0,
						Limit:     10,
						BatchSize: 5,
					},
				},
				searchFunc: func(_ context.Context, _ *data.SearchQuery) ([]*TestItem, error) {
					return nil, errors.New("search failed")
				},
				expectedItems: 0,
				expectedError: true,
			},
			{
				name: "empty search results",
				query: &data.SearchQuery{
					Query:  "empty",
					Fields: map[string]any{},
					Pagination: &data.Paginator{
						Offset:    0,
						Limit:     10,
						BatchSize: 5,
					},
				},
				searchFunc: func(_ context.Context, _ *data.SearchQuery) ([]*TestItem, error) {
					return []*TestItem{}, nil
				},
				expectedItems: 0,
				expectedError: false,
			},
		}

		s.runStableSearchTests(t, depOpt, testCases)
	})
}

// runStableSearchTests is a helper function to reduce complexity in TestStableSearchWithDependencies.
func (s *SearchTestSuite) runStableSearchTests(t *testing.T, depOpt *definition.DependancyOption, testCases []struct {
	name          string
	query         *data.SearchQuery
	searchFunc    func(ctx context.Context, query *data.SearchQuery) ([]*TestItem, error)
	expectedItems int
	expectedError bool
}) {
	for _, tc := range testCases {
		t.Run(tc.name, func(tt *testing.T) {
			ctx := context.Background()

			// Create a dbPool with the test dependencies
			ctx, svc := frame.NewServiceWithContext(ctx, "search-test",
				frame.WithDatastoreConnection(depOpt.ByIsDatabase(ctx).GetDS(ctx).String(), false),
				frame.WithRegisterPublisher("test-queue", depOpt.ByIsQueue(ctx).GetDS(ctx).String()),
			)
			defer svc.Stop(ctx)

			// Execute StableSearch
			jobPipe, err := data.StableSearch(ctx, svc.WorkManager(), tc.query, tc.searchFunc)

			if tc.expectedError {
				s.handleErrorCase(tt, err, jobPipe)
			} else {
				s.handleSuccessCase(tt, err, jobPipe, tc.expectedItems)
			}
		})
	}
}

// handleErrorCase handles error test cases.
func (s *SearchTestSuite) handleErrorCase(t *testing.T, err error, jobPipe workerpool.JobResultPipe[[]*TestItem]) {
	if err == nil {
		// Wait for job to complete and check for errors
		var hasError bool
		for result := range jobPipe.ResultChan() {
			if result.IsError() {
				hasError = true
				break
			}
		}
		require.True(t, hasError, "Expected error but none occurred")
	} else {
		require.Error(t, err)
	}
}

// handleSuccessCase handles successful test cases.
func (s *SearchTestSuite) handleSuccessCase(
	t *testing.T,
	err error,
	jobPipe workerpool.JobResultPipe[[]*TestItem],
	expectedItems int,
) {
	require.NoError(t, err)
	require.NotNil(t, jobPipe)

	// Collect all results
	var allItems []*TestItem
	for result := range jobPipe.ResultChan() {
		require.False(t, result.IsError(), "Unexpected error: %v", result.Error())
		allItems = append(allItems, result.Item()...)
	}

	assert.Len(t, allItems, expectedItems)
}

// createMultiBatchSearchFunc creates a search function that returns data in multiple batches.
func createMultiBatchSearchFunc() func(ctx context.Context, query *data.SearchQuery) ([]*TestItem, error) {
	callCount := 0
	return func(_ context.Context, query *data.SearchQuery) ([]*TestItem, error) {
		callCount++

		// Simulate pagination by returning different data based on offset
		batchSize := query.Pagination.BatchSize
		if query.Pagination.Offset >= 25 {
			return []*TestItem{}, nil // No more data
		}

		items := make([]*TestItem, 0, batchSize)
		for i := range batchSize {
			itemID := query.Pagination.Offset + i + 1
			if itemID > 25 {
				break
			}
			items = append(items, &TestItem{
				ID:   strconv.Itoa(itemID),
				Name: fmt.Sprintf("Item %d", itemID),
			})
		}

		return items, nil
	}
}

// TestProfileIDHandling tests search queries with different ProfileID types.
func (s *SearchTestSuite) TestProfileIDHandling() {
	testCases := []struct {
		name      string
		query     string
		fields    map[string]any
		profileID any
	}{
		{
			name:      "UUID profile ID",
			query:     "test query",
			fields:    map[string]any{"category": "test"},
			profileID: "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:      "string profile ID",
			query:     "another query",
			fields:    map[string]any{"type": "user"},
			profileID: "user_profile_123",
		},
		{
			name:      "numeric profile ID",
			query:     "numeric test",
			fields:    map[string]any{"status": "active"},
			profileID: 12345,
		},
		{
			name:      "nil profile ID",
			query:     "nil test",
			fields:    map[string]any{"test": true},
			profileID: nil,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			result := data.NewSearchQuery(tc.query, tc.fields, 0, 10)
			s.NotNil(result)

			s.Equal(tc.query, result.Query)
			s.Equal(tc.fields, result.Fields)
			// Note: ProfileID is not set by NewSearchQuery, so we just verify the basic query structure
		})
	}
}

// TestPaginatorEdgeCases tests complex edge cases for Paginator behavior.
func (s *SearchTestSuite) TestPaginatorEdgeCases() {
	testCases := []struct {
		name           string
		paginator      *data.Paginator
		operations     []paginatorOperation
		expectedStates []paginatorState
	}{
		{
			name: "multiple stop calls with varying loads",
			paginator: &data.Paginator{
				Offset:    0,
				Limit:     20,
				BatchSize: 5,
			},
			operations: []paginatorOperation{
				{opType: "stop", loadedCount: 5},
				{opType: "stop", loadedCount: 3},
				{opType: "stop", loadedCount: 5},
				{opType: "stop", loadedCount: 0},
			},
			expectedStates: []paginatorState{
				{shouldStop: false, offset: 5, batchSize: 5},  // 5 == 5, so false
				{shouldStop: true, offset: 8, batchSize: 5},   // 3 < 5, so true
				{shouldStop: false, offset: 13, batchSize: 5}, // 5 == 5, so false
				{shouldStop: true, offset: 13, batchSize: 5},  // 0 < 5, so true
			},
		},
		{
			name: "boundary conditions",
			paginator: &data.Paginator{
				Offset:    18,
				Limit:     20,
				BatchSize: 5,
			},
			operations: []paginatorOperation{
				{opType: "canLoad"},
				{opType: "stop", loadedCount: 2},
				{opType: "canLoad"},
			},
			expectedStates: []paginatorState{
				{canLoad: true},
				{shouldStop: false, offset: 20, batchSize: 0},
				{canLoad: false},
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			paginator := tc.paginator

			for i, op := range tc.operations {
				expected := tc.expectedStates[i]

				switch op.opType {
				case "canLoad":
					result := paginator.CanLoad()
					s.Equal(expected.canLoad, result)
				case "stop":
					result := paginator.Stop(op.loadedCount)
					s.Equal(expected.shouldStop, result)
					s.Equal(expected.offset, paginator.Offset)
					s.Equal(expected.batchSize, paginator.BatchSize)
				}
			}
		})
	}
}

// TestStableSearchConcurrency tests concurrent access to StableSearch.
func (s *SearchTestSuite) TestStableSearchConcurrency() {
	depOptions := []*definition.DependancyOption{
		definition.NewDependancyOption("postgres_nats", "test", s.Resources()),
	}

	frametests.WithTestDependancies(s.T(), depOptions, func(t *testing.T, depOpt *definition.DependancyOption) {
		ctx := context.Background()

		// Create a dbPool with the test dependencies
		ctx, svc := frame.NewServiceWithContext(ctx, "search-test",
			frame.WithDatastoreConnection(depOpt.ByIsDatabase(ctx).GetDS(ctx).String(), false),
			frame.WithRegisterPublisher("test-queue", depOpt.ByIsQueue(ctx).GetDS(ctx).String()),
		)
		defer svc.Stop(ctx)

		const numConcurrentSearches = 10
		results := make(chan int, numConcurrentSearches)
		concurrentErrors := make(chan error, numConcurrentSearches)

		searchFunc := func(_ context.Context, _ *data.SearchQuery) ([]*TestItem, error) {
			// Simulate some processing time
			return []*TestItem{
				{ID: "1", Name: "Item 1"},
			}, nil
		}

		for i := range numConcurrentSearches {
			go func(searchID int) {
				query := &data.SearchQuery{
					Query:  "concurrent test",
					Fields: map[string]any{"search_id": searchID},
					Pagination: &data.Paginator{
						Offset:    0,
						Limit:     10,
						BatchSize: 5,
					},
				}

				jobPipe, err := data.StableSearch(ctx, svc.WorkManager(), query, searchFunc)
				if err != nil {
					concurrentErrors <- err
					return
				}

				itemCount := 0
				for result := range jobPipe.ResultChan() {
					if result.IsError() {
						concurrentErrors <- result.Error()
						return
					}
					itemCount += len(result.Item())
				}

				results <- itemCount
			}(i)
		}

		// Collect results
		successCount := 0
		errorCount := 0

		for range numConcurrentSearches {
			select {
			case count := <-results:
				successCount++
				assert.Positive(t, count)
			case err := <-concurrentErrors:
				errorCount++
				t.Logf("Concurrent search error: %v", err)
			}
		}

		// All searches should succeed
		assert.Equal(t, numConcurrentSearches, successCount)
		assert.Equal(t, 0, errorCount)
	})
}

// TestStableSearchMemoryManagement tests memory handling with large result sets.
func (s *SearchTestSuite) TestStableSearchMemoryManagement() {
	depOptions := []*definition.DependancyOption{
		definition.NewDependancyOption("postgres_nats", "test", s.Resources()),
	}

	frametests.WithTestDependancies(s.T(), depOptions, func(t *testing.T, depOpt *definition.DependancyOption) {
		ctx := context.Background()

		// Create a dbPool with the test dependencies
		ctx, svc := frame.NewServiceWithContext(ctx, "search-test",
			frame.WithDatastoreConnection(depOpt.ByIsDatabase(ctx).GetDS(ctx).String(), false),
			frame.WithRegisterPublisher("test-queue", depOpt.ByIsQueue(ctx).GetDS(ctx).String()),
		)
		defer svc.Stop(ctx)

		// Test with large result sets to check memory handling
		largeSearchFunc := func(_ context.Context, query *data.SearchQuery) ([]*TestItem, error) {
			items := make([]*TestItem, query.Pagination.BatchSize)
			for i := range query.Pagination.BatchSize {
				items[i] = &TestItem{
					ID:   fmt.Sprintf("item_%d_%d", query.Pagination.Offset, i),
					Name: fmt.Sprintf("Large Item %d", query.Pagination.Offset+i),
				}
			}
			return items, nil
		}

		query := &data.SearchQuery{
			Query:  "memory test",
			Fields: map[string]any{"type": "large"},
			Pagination: &data.Paginator{
				Offset:    0,
				Limit:     1000, // Large limit
				BatchSize: 50,   // Reasonable batch size
			},
		}

		jobPipe, err := data.StableSearch(ctx, svc.WorkManager(), query, largeSearchFunc)
		require.NoError(t, err)
		require.NotNil(t, jobPipe)

		totalItems := 0
		batchCount := 0

		for result := range jobPipe.ResultChan() {
			require.False(t, result.IsError(), "Unexpected error: %v", result.Error())

			batch := result.Item()
			totalItems += len(batch)
			batchCount++

			// Verify batch size constraints
			assert.LessOrEqual(t, len(batch), 50, "Batch size should not exceed BatchSize")
		}

		// Verify total results
		assert.Equal(t, 1000, totalItems)
		assert.Equal(t, 20, batchCount) // 1000 / 50 = 20 batches
	})
}

// TestFieldTypeValidation tests various field types in search queries.
func (s *SearchTestSuite) TestFieldTypeValidation() {
	testCases := []struct {
		name   string
		fields map[string]any
	}{
		{
			name: "string fields",
			fields: map[string]any{
				"name":        "John Doe",
				"email":       "john@example.com",
				"description": "A test user",
			},
		},
		{
			name: "numeric fields",
			fields: map[string]any{
				"age":    30,
				"score":  95.5,
				"count":  int64(1000),
				"rating": float32(4.5),
			},
		},
		{
			name: "boolean fields",
			fields: map[string]any{
				"active":    true,
				"verified":  false,
				"premium":   true,
				"suspended": false,
			},
		},
		{
			name: "complex fields",
			fields: map[string]any{
				"tags":     []string{"tag1", "tag2", "tag3"},
				"metadata": map[string]string{"key1": "value1", "key2": "value2"},
				"settings": map[string]any{"theme": "dark", "notifications": true},
			},
		},
		{
			name: "mixed field types",
			fields: map[string]any{
				"id":       "user_123",
				"age":      25,
				"active":   true,
				"tags":     []string{"user", "premium"},
				"metadata": map[string]any{"created": "2023-01-01", "version": 2},
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			result := data.NewSearchQuery("test query", tc.fields, 0, 10)

			s.NotNil(result)
			s.Equal(tc.fields, result.Fields)
			s.Equal("test query", result.Query)
		})
	}
}

// TestPaginatorStressTesting tests paginator under extreme conditions.
func (s *SearchTestSuite) TestPaginatorStressTesting() {
	testCases := []struct {
		name      string
		limit     int
		batchSize int
		expected  int // expected number of batches
	}{
		{
			name:      "very large limit with small batch",
			limit:     10000,
			batchSize: 1,
			expected:  10000,
		},
		{
			name:      "small limit with large batch",
			limit:     5,
			batchSize: 100,
			expected:  1,
		},
		{
			name:      "equal limit and batch size",
			limit:     50,
			batchSize: 50,
			expected:  1,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			paginator := &data.Paginator{
				Offset:    0,
				Limit:     tc.limit,
				BatchSize: tc.batchSize,
			}

			batchCount := 0
			for paginator.CanLoad() {
				// Simulate loading the current batch size
				loadedCount := minInt(tc.batchSize, tc.limit-paginator.Offset)
				if paginator.Stop(loadedCount) {
					break
				}
				batchCount++

				// Safety check to prevent infinite loops in tests
				if batchCount > tc.expected+1 {
					s.Fail("Too many batches, possible infinite loop")
					break
				}
			}

			s.Equal(tc.expected, batchCount)
		})
	}
}

// TestStableSearchErrorRecovery tests error recovery scenarios.
func (s *SearchTestSuite) TestStableSearchErrorRecovery() {
	depOptions := []*definition.DependancyOption{
		definition.NewDependancyOption("postgres_nats", "test", s.Resources()),
	}

	frametests.WithTestDependancies(s.T(), depOptions, func(t *testing.T, depOpt *definition.DependancyOption) {
		testCases := []struct {
			name       string
			searchFunc func(ctx context.Context, query *data.SearchQuery) ([]*TestItem, error)
			expectErr  bool
		}{
			{
				name: "context cancellation",
				searchFunc: func(_ context.Context, _ *data.SearchQuery) ([]*TestItem, error) {
					// Cancel context during search
					return nil, context.Canceled
				},
				expectErr: true,
			},
			{
				name: "timeout error",
				searchFunc: func(_ context.Context, _ *data.SearchQuery) ([]*TestItem, error) {
					return nil, context.DeadlineExceeded
				},
				expectErr: true,
			},
			{
				name: "custom error",
				searchFunc: func(_ context.Context, _ *data.SearchQuery) ([]*TestItem, error) {
					return nil, errors.New("database connection failed")
				},
				expectErr: true,
			},
			{
				name: "panic recovery simulation",
				searchFunc: func(_ context.Context, _ *data.SearchQuery) ([]*TestItem, error) {
					// Simulate a recoverable error condition
					return nil, errors.New("panic: runtime error")
				},
				expectErr: true,
			},
		}

		s.runErrorRecoveryTests(t, depOpt, testCases)
	})
}

// runErrorRecoveryTests is a helper function to reduce complexity in TestStableSearchErrorRecovery.
func (s *SearchTestSuite) runErrorRecoveryTests(t *testing.T, depOpt *definition.DependancyOption, testCases []struct {
	name       string
	searchFunc func(ctx context.Context, query *data.SearchQuery) ([]*TestItem, error)
	expectErr  bool
}) {
	for _, tc := range testCases {
		t.Run(tc.name, func(tt *testing.T) {
			ctx := context.Background()

			// Create a dbPool with the test dependencies
			ctx, svc := frame.NewServiceWithContext(ctx, "search-test",
				frame.WithDatastoreConnection(depOpt.ByIsDatabase(ctx).GetDS(ctx).String(), false),
				frame.WithRegisterPublisher("test-queue", depOpt.ByIsQueue(ctx).GetDS(ctx).String()),
			)
			defer svc.Stop(ctx)

			query := &data.SearchQuery{
				Query:  "error test",
				Fields: map[string]any{"test": "error"},
				Pagination: &data.Paginator{
					Offset:    0,
					Limit:     10,
					BatchSize: 5,
				},
			}

			jobPipe, err := data.StableSearch(ctx, svc.WorkManager(), query, tc.searchFunc)

			s.validateErrorTestResult(tt, tc.expectErr, err, jobPipe)
		})
	}
}

// validateErrorTestResult validates the result of error test cases.
func (s *SearchTestSuite) validateErrorTestResult(
	t *testing.T,
	expectErr bool,
	err error,
	jobPipe workerpool.JobResultPipe[[]*TestItem],
) {
	if !expectErr {
		require.NoError(t, err)
		require.NotNil(t, jobPipe)
		return
	}

	// For error cases, we might get an error immediately or through the job
	if err != nil {
		require.Error(t, err)
		return
	}

	// Wait for job to complete and check for errors
	var hasError bool
	for result := range jobPipe.ResultChan() {
		if result.IsError() {
			hasError = true
			break
		}
	}
	require.True(t, hasError, "Expected error but none occurred")
}

type paginatorOperation struct {
	opType      string
	loadedCount int
}

type paginatorState struct {
	canLoad    bool
	shouldStop bool
	offset     int
	batchSize  int
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type TestItem struct {
	ID   string
	Name string
}
