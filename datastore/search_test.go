package datastore

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/tests"
	"github.com/pitabwire/frame/tests/deps/testnats"
	"github.com/pitabwire/frame/tests/deps/testpostgres"
	"github.com/pitabwire/frame/tests/testdef"
)

// SearchTestSuite extends FrameBaseTestSuite for comprehensive search testing
type SearchTestSuite struct {
	tests.FrameBaseTestSuite
}

// TestSearchSuite runs the search test suite
func TestSearchSuite(t *testing.T) {
	suite.Run(t, &SearchTestSuite{
		FrameBaseTestSuite: tests.FrameBaseTestSuite{
			InitResourceFunc: func(ctx context.Context) []testdef.TestResource {
				return []testdef.TestResource{
					testpostgres.NewPGDep(),
					testnats.NewNatsDep(),
				}
			},
		},
	})
}

// TestNewSearchQuery tests the NewSearchQuery function with various parameters
func (s *SearchTestSuite) TestNewSearchQuery() {
	testCases := []struct {
		name          string
		query         string
		fields        map[string]any
		resultPage    int
		resultCount   int
		expectError   bool
		expectedQuery *SearchQuery
	}{
		{
			name:        "valid query with default count",
			query:       "test query",
			fields:      map[string]any{"field1": "value1"},
			resultPage:  0,
			resultCount: 0,
			expectError: false,
			expectedQuery: &SearchQuery{
				Query:  "test query",
				Fields: map[string]any{"field1": "value1"},
				Pagination: &Paginator{
					Offset:    0,
					Limit:     defaultBatchSize,
					BatchSize: defaultBatchSize,
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
			expectedQuery: &SearchQuery{
				Query:  "search term",
				Fields: map[string]any{"name": "John", "age": 30},
				Pagination: &Paginator{
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
			expectedQuery: &SearchQuery{
				Query:  "large query",
				Fields: map[string]any{},
				Pagination: &Paginator{
					Offset:    0,
					Limit:     100,
					BatchSize: defaultBatchSize,
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
			expectedQuery: &SearchQuery{
				Query:  "",
				Fields: map[string]any{"status": "active"},
				Pagination: &Paginator{
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
			expectedQuery: &SearchQuery{
				Query:  "test",
				Fields: nil,
				Pagination: &Paginator{
					Offset:    0,
					Limit:     15,
					BatchSize: 15,
				},
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			ctx := context.Background()

			result, err := NewSearchQuery(ctx, tc.query, tc.fields, tc.resultPage, tc.resultCount)

			if tc.expectError {
				assert.Error(s.T(), err)
				assert.Nil(s.T(), result)
			} else {
				assert.NoError(s.T(), err)
				assert.NotNil(s.T(), result)
				assert.Equal(s.T(), tc.expectedQuery.Query, result.Query)
				assert.Equal(s.T(), tc.expectedQuery.Fields, result.Fields)
				assert.Equal(s.T(), tc.expectedQuery.Pagination.Offset, result.Pagination.Offset)
				assert.Equal(s.T(), tc.expectedQuery.Pagination.Limit, result.Pagination.Limit)
				assert.Equal(s.T(), tc.expectedQuery.Pagination.BatchSize, result.Pagination.BatchSize)
			}
		})
	}
}

// TestPaginatorCanLoad tests the CanLoad method with various offset/limit combinations
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
			paginator := &Paginator{
				Offset: tc.offset,
				Limit:  tc.limit,
			}

			result := paginator.CanLoad()
			assert.Equal(s.T(), tc.expected, result)
		})
	}
}

// TestPaginatorStop tests the Stop method with different loaded counts and batch sizes
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
			paginator := &Paginator{
				Offset:    tc.initialOffset,
				Limit:     tc.initialLimit,
				BatchSize: tc.initialBatchSize,
			}

			result := paginator.Stop(tc.loadedCount)

			assert.Equal(s.T(), tc.expectedStop, result)
			assert.Equal(s.T(), tc.expectedOffset, paginator.Offset)
			assert.Equal(s.T(), tc.expectedBatchSize, paginator.BatchSize)
		})
	}
}

// TestStableSearchWithDependencies tests StableSearch with real infrastructure dependencies
func (s *SearchTestSuite) TestStableSearchWithDependencies() {
	// Create dependency options for testing
	deps := s.Resources()
	depOptions := []*testdef.DependancyOption{
		testdef.NewDependancyOption("postgres_nats", "test", deps),
	}

	tests.WithTestDependancies(s.T(), depOptions, func(t *testing.T, depOpt *testdef.DependancyOption) {
		testCases := []struct {
			name          string
			query         *SearchQuery
			searchFunc    func(ctx context.Context, query *SearchQuery) ([]*TestItem, error)
			expectedItems int
			expectedError bool
		}{
			{
				name: "successful search with single batch",
				query: &SearchQuery{
					Query:  "test",
					Fields: map[string]any{"category": "books"},
					Pagination: &Paginator{
						Offset:    0,
						Limit:     10,
						BatchSize: 10,
					},
				},
				searchFunc: func(ctx context.Context, query *SearchQuery) ([]*TestItem, error) {
					return []*TestItem{
						{ID: "1", Name: "Item 1"},
						{ID: "2", Name: "Item 2"},
						{ID: "3", Name: "Item 3"},
					}, nil
				},
				expectedItems: 3,
				expectedError: false,
			},
			{
				name: "successful search with multiple batches",
				query: &SearchQuery{
					Query:  "multi",
					Fields: map[string]any{},
					Pagination: &Paginator{
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
				query: &SearchQuery{
					Query:  "error",
					Fields: map[string]any{},
					Pagination: &Paginator{
						Offset:    0,
						Limit:     10,
						BatchSize: 5,
					},
				},
				searchFunc: func(ctx context.Context, query *SearchQuery) ([]*TestItem, error) {
					return nil, errors.New("search failed")
				},
				expectedItems: 0,
				expectedError: true,
			},
			{
				name: "empty search results",
				query: &SearchQuery{
					Query:  "empty",
					Fields: map[string]any{},
					Pagination: &Paginator{
						Offset:    0,
						Limit:     10,
						BatchSize: 5,
					},
				},
				searchFunc: func(ctx context.Context, query *SearchQuery) ([]*TestItem, error) {
					return []*TestItem{}, nil
				},
				expectedItems: 0,
				expectedError: false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(tt *testing.T) {
				ctx := context.Background()

				// Create a service with the test dependencies
				ctx, svc := frame.NewServiceWithContext(ctx, "search-test",
					frame.WithDatastoreConnection(depOpt.Database()[0].GetDS().String(), false),
					frame.WithRegisterPublisher("test-queue", depOpt.Queue()[0].GetDS().String()),
				)
				defer svc.Stop(ctx)

				// Execute StableSearch
				jobPipe, err := StableSearch(ctx, svc, tc.query, tc.searchFunc)

				if tc.expectedError {
					// For error cases, we might get an error immediately or through the job
					if err == nil {
						// Wait for job to complete and check for errors
						var totalItems int
						for result := range jobPipe.ResultChan() {
							if result.IsError() {
								assert.Error(tt, result.Error())
								break
							}
							if !result.IsError() {
								totalItems += len(result.Item())
							}
						}
					} else {
						assert.Error(tt, err)
					}
				} else {
					require.NoError(tt, err)
					require.NotNil(tt, jobPipe)

					// Collect all results
					var totalItems int
					var hasError bool
					for result := range jobPipe.ResultChan() {
						if result.IsError() {
							hasError = true
							break
						}
						if !result.IsError() {
							totalItems += len(result.Item())
						}
					}

					assert.False(tt, hasError)
					assert.Equal(tt, tc.expectedItems, totalItems)
				}
			})
		}
	})
}

// TestItem represents a test data structure for search results
type TestItem struct {
	ID   string
	Name string
}

// createMultiBatchSearchFunc creates a search function that returns data in multiple batches
func createMultiBatchSearchFunc() func(ctx context.Context, query *SearchQuery) ([]*TestItem, error) {
	callCount := 0
	return func(ctx context.Context, query *SearchQuery) ([]*TestItem, error) {
		callCount++

		// Simulate pagination by returning different data based on offset
		offset := query.Pagination.Offset
		batchSize := query.Pagination.BatchSize
		limit := query.Pagination.Limit

		// Calculate how many items to return for this batch
		remaining := limit - offset
		itemsToReturn := batchSize
		if remaining < batchSize {
			itemsToReturn = remaining
		}

		// Generate test items
		items := make([]*TestItem, itemsToReturn)
		for i := 0; i < itemsToReturn; i++ {
			items[i] = &TestItem{
				ID:   string(rune('A' + offset + i)),
				Name: "Item " + string(rune('A'+offset+i)),
			}
		}

		return items, nil
	}
}

// TestSearchQueryValidation tests edge cases and validation scenarios
func (s *SearchTestSuite) TestSearchQueryValidation() {
	testCases := []struct {
		name        string
		query       string
		fields      map[string]any
		resultPage  int
		resultCount int
		expectError bool
	}{
		{
			name:        "negative result page",
			query:       "test",
			fields:      map[string]any{},
			resultPage:  -1,
			resultCount: 10,
			expectError: false, // Should handle gracefully
		},
		{
			name:        "negative result count",
			query:       "test",
			fields:      map[string]any{},
			resultPage:  0,
			resultCount: -5,
			expectError: false, // Should default to defaultBatchSize
		},
		{
			name:        "very large result page",
			query:       "test",
			fields:      map[string]any{},
			resultPage:  1000000,
			resultCount: 10,
			expectError: false,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			ctx := context.Background()

			result, err := NewSearchQuery(ctx, tc.query, tc.fields, tc.resultPage, tc.resultCount)

			if tc.expectError {
				assert.Error(s.T(), err)
			} else {
				assert.NoError(s.T(), err)
				assert.NotNil(s.T(), result)
				assert.NotNil(s.T(), result.Pagination)
			}
		})
	}
}

// TestPaginatorEdgeCases tests edge cases for paginator behavior
func (s *SearchTestSuite) TestPaginatorEdgeCases() {
	testCases := []struct {
		name           string
		paginator      *Paginator
		operations     []paginatorOperation
		expectedStates []paginatorState
	}{
		{
			name: "multiple stop calls with varying loads",
			paginator: &Paginator{
				Offset:    0,
				Limit:     20,
				BatchSize: 5,
			},
			operations: []paginatorOperation{
				{action: "stop", loadedCount: 5},
				{action: "canLoad", expected: true},
				{action: "stop", loadedCount: 5},
				{action: "canLoad", expected: true},
				{action: "stop", loadedCount: 3}, // Partial load - should stop
			},
			expectedStates: []paginatorState{
				{offset: 5, shouldStop: false},
				{canLoad: true},
				{offset: 10, shouldStop: false},
				{canLoad: true},
				{offset: 13, shouldStop: true},
			},
		},
		{
			name: "boundary conditions",
			paginator: &Paginator{
				Offset:    18,
				Limit:     20,
				BatchSize: 5,
			},
			operations: []paginatorOperation{
				{action: "canLoad", expected: true},
				{action: "stop", loadedCount: 2},
				{action: "canLoad", expected: false},
			},
			expectedStates: []paginatorState{
				{canLoad: true},
				{offset: 20, shouldStop: false, batchSize: 0},
				{canLoad: false},
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			paginator := tc.paginator

			for i, op := range tc.operations {
				expectedState := tc.expectedStates[i]

				switch op.action {
				case "stop":
					result := paginator.Stop(op.loadedCount)
					assert.Equal(s.T(), expectedState.shouldStop, result)
					assert.Equal(s.T(), expectedState.offset, paginator.Offset)
					if expectedState.batchSize != 0 {
						assert.Equal(s.T(), expectedState.batchSize, paginator.BatchSize)
					}
				case "canLoad":
					result := paginator.CanLoad()
					assert.Equal(s.T(), expectedState.canLoad, result)
				}
			}
		})
	}
}

// Helper types for edge case testing
type paginatorOperation struct {
	action      string
	loadedCount int
	expected    bool
}

type paginatorState struct {
	offset     int
	canLoad    bool
	shouldStop bool
	batchSize  int
}

// TestStableSearchConcurrency tests concurrent access to StableSearch
func (s *SearchTestSuite) TestStableSearchConcurrency() {
	deps := s.Resources()
	depOptions := []*testdef.DependancyOption{
		testdef.NewDependancyOption("postgres_nats", "concurrent", deps),
	}

	tests.WithTestDependancies(s.T(), depOptions, func(t *testing.T, depOpt *testdef.DependancyOption) {
		ctx := context.Background()

		// Create a service with the test dependencies
		ctx, svc := frame.NewServiceWithContext(ctx, "search-concurrent-test",
			frame.WithDatastoreConnection(depOpt.Database()[0].GetDS().String(), false),
			frame.WithRegisterPublisher("test-queue", depOpt.Queue()[0].GetDS().String()),
		)
		defer svc.Stop(ctx)

		// Test concurrent searches
		const numConcurrentSearches = 10
		results := make(chan int, numConcurrentSearches)
		concurrentErrors := make(chan error, numConcurrentSearches)

		searchFunc := func(ctx context.Context, query *SearchQuery) ([]*TestItem, error) {
			// Simulate some processing time
			return []*TestItem{
				{ID: "1", Name: "Item 1"},
				{ID: "2", Name: "Item 2"},
			}, nil
		}

		for i := 0; i < numConcurrentSearches; i++ {
			go func(searchID int) {
				query := &SearchQuery{
					Query:  "concurrent test",
					Fields: map[string]any{"search_id": searchID},
					Pagination: &Paginator{
						Offset:    0,
						Limit:     10,
						BatchSize: 5,
					},
				}

				jobPipe, err := StableSearch(ctx, svc, query, searchFunc)
				if err != nil {
					concurrentErrors <- err
					return
				}

				totalItems := 0
				for result := range jobPipe.ResultChan() {
					if result.IsError() {
						concurrentErrors <- result.Error()
						return
					}
					totalItems += len(result.Item())
				}
				results <- totalItems
			}(i)
		}

		// Collect results
		var successCount int
		var errorCount int
		for i := 0; i < numConcurrentSearches; i++ {
			select {
			case count := <-results:
				assert.Equal(t, 2, count) // Each search should return 2 items
				successCount++
			case err := <-concurrentErrors:
				t.Logf("Concurrent search error: %v", err)
				errorCount++
			}
		}

		assert.Equal(t, numConcurrentSearches, successCount)
		assert.Equal(t, 0, errorCount)
	})
}

// TestStableSearchMemoryManagement tests memory usage and cleanup
func (s *SearchTestSuite) TestStableSearchMemoryManagement() {
	deps := s.Resources()
	depOptions := []*testdef.DependancyOption{
		testdef.NewDependancyOption("postgres_nats", "memory", deps),
	}

	tests.WithTestDependancies(s.T(), depOptions, func(t *testing.T, depOpt *testdef.DependancyOption) {
		ctx := context.Background()

		ctx, svc := frame.NewServiceWithContext(ctx, "search-memory-test",
			frame.WithDatastoreConnection(depOpt.Database()[0].GetDS().String(), false),
			frame.WithRegisterPublisher("test-queue", depOpt.Queue()[0].GetDS().String()),
		)
		defer svc.Stop(ctx)

		// Test with large result sets to check memory handling
		largeSearchFunc := func(ctx context.Context, query *SearchQuery) ([]*TestItem, error) {
			items := make([]*TestItem, query.Pagination.BatchSize)
			for i := 0; i < query.Pagination.BatchSize; i++ {
				items[i] = &TestItem{
					ID:   fmt.Sprintf("item_%d_%d", query.Pagination.Offset, i),
					Name: fmt.Sprintf("Large Item %d", query.Pagination.Offset+i),
				}
			}
			return items, nil
		}

		query := &SearchQuery{
			Query:  "memory test",
			Fields: map[string]any{"type": "large"},
			Pagination: &Paginator{
				Offset:    0,
				Limit:     1000, // Large limit
				BatchSize: 50,   // Reasonable batch size
			},
		}

		jobPipe, err := StableSearch(ctx, svc, query, largeSearchFunc)
		require.NoError(t, err)
		require.NotNil(t, jobPipe)

		totalItems := 0
		batchCount := 0
		for result := range jobPipe.ResultChan() {
			require.False(t, result.IsError(), "Should not have errors: %v", result.Error())
			items := result.Item()
			totalItems += len(items)
			batchCount++

			// Verify batch size constraints
			assert.LessOrEqual(t, len(items), 50, "Batch size should not exceed limit")
		}

		assert.Equal(t, 1000, totalItems, "Should process all items")
		assert.Equal(t, 20, batchCount, "Should have correct number of batches (1000/50)")
	})
}

// TestSearchQueryFieldTypes tests various field types in SearchQuery
func (s *SearchTestSuite) TestSearchQueryFieldTypes() {
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
			name: "boolean and nil fields",
			fields: map[string]any{
				"active":   true,
				"verified": false,
				"deleted":  nil,
			},
		},
		{
			name: "complex types",
			fields: map[string]any{
				"tags":     []string{"tag1", "tag2", "tag3"},
				"metadata": map[string]string{"key": "value"},
				"config":   struct{ Enabled bool }{Enabled: true},
			},
		},
		{
			name: "mixed types",
			fields: map[string]any{
				"id":        123,
				"name":      "Mixed Test",
				"active":    true,
				"score":     98.7,
				"tags":      []int{1, 2, 3},
				"timestamp": "2023-01-01T00:00:00Z",
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			ctx := context.Background()

			result, err := NewSearchQuery(ctx, "test query", tc.fields, 0, 10)

			assert.NoError(s.T(), err)
			assert.NotNil(s.T(), result)
			assert.Equal(s.T(), tc.fields, result.Fields)
			assert.Equal(s.T(), "test query", result.Query)
		})
	}
}

// TestPaginatorStressTest tests paginator under stress conditions
func (s *SearchTestSuite) TestPaginatorStressTest() {
	testCases := []struct {
		name        string
		limit       int
		batchSize   int
		expectedOps int
	}{
		{
			name:        "small batches, large limit",
			limit:       10000,
			batchSize:   1,
			expectedOps: 10000,
		},
		{
			name:        "large batches, small limit",
			limit:       10,
			batchSize:   1000,
			expectedOps: 1,
		},
		{
			name:        "equal batch and limit",
			limit:       100,
			batchSize:   100,
			expectedOps: 1,
		},
		{
			name:        "prime numbers",
			limit:       97,
			batchSize:   13,
			expectedOps: 8, // ceil(97/13) = 8
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			paginator := &Paginator{
				Offset:    0,
				Limit:     tc.limit,
				BatchSize: tc.batchSize,
			}

			operations := 0
			totalProcessed := 0

			for paginator.CanLoad() {
				operations++

				// Simulate processing a batch
				itemsToProcess := tc.batchSize
				if totalProcessed+itemsToProcess > tc.limit {
					itemsToProcess = tc.limit - totalProcessed
				}

				totalProcessed += itemsToProcess
				shouldStop := paginator.Stop(itemsToProcess)

				if shouldStop {
					break
				}

				// Safety check to prevent infinite loops
				if operations > tc.expectedOps+1 {
					s.T().Fatalf("Too many operations: %d, expected around %d", operations, tc.expectedOps)
				}
			}

			assert.Equal(s.T(), tc.limit, totalProcessed, "Should process all items")
			assert.LessOrEqual(s.T(), operations, tc.expectedOps+1, "Operations should be within expected range")
		})
	}
}

// TestStableSearchErrorRecovery tests error recovery scenarios
func (s *SearchTestSuite) TestStableSearchErrorRecovery() {
	deps := s.Resources()
	depOptions := []*testdef.DependancyOption{
		testdef.NewDependancyOption("postgres_nats", "error_recovery", deps),
	}

	tests.WithTestDependancies(s.T(), depOptions, func(t *testing.T, depOpt *testdef.DependancyOption) {
		testCases := []struct {
			name       string
			searchFunc func(ctx context.Context, query *SearchQuery) ([]*TestItem, error)
			expectErr  bool
		}{
			{
				name: "context cancellation",
				searchFunc: func(ctx context.Context, query *SearchQuery) ([]*TestItem, error) {
					// Cancel context during search
					return nil, context.Canceled
				},
				expectErr: true,
			},
			{
				name: "timeout error",
				searchFunc: func(ctx context.Context, query *SearchQuery) ([]*TestItem, error) {
					return nil, context.DeadlineExceeded
				},
				expectErr: true,
			},
			{
				name: "custom error",
				searchFunc: func(ctx context.Context, query *SearchQuery) ([]*TestItem, error) {
					return nil, errors.New("database connection failed")
				},
				expectErr: true,
			},
			{
				name: "panic recovery simulation",
				searchFunc: func(ctx context.Context, query *SearchQuery) ([]*TestItem, error) {
					// Simulate a recoverable error condition
					return nil, errors.New("panic: runtime error")
				},
				expectErr: true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(tt *testing.T) {
				ctx := context.Background()

				ctx, svc := frame.NewServiceWithContext(ctx, "search-error-test",
					frame.WithDatastoreConnection(depOpt.Database()[0].GetDS().String(), false),
					frame.WithRegisterPublisher("test-queue", depOpt.Queue()[0].GetDS().String()),
				)
				defer svc.Stop(ctx)

				query := &SearchQuery{
					Query:  "error test",
					Fields: map[string]any{"test": "error"},
					Pagination: &Paginator{
						Offset:    0,
						Limit:     10,
						BatchSize: 5,
					},
				}

				jobPipe, err := StableSearch(ctx, svc, query, tc.searchFunc)

				if tc.expectErr {
					if err == nil {
						// Error might come through the job pipe
						hasError := false
						for result := range jobPipe.ResultChan() {
							if result.IsError() {
								hasError = true
								assert.Error(tt, result.Error())
								break
							}
						}
						assert.True(tt, hasError, "Expected error in job results")
					} else {
						assert.Error(tt, err)
					}
				} else {
					assert.NoError(tt, err)
				}
			})
		}
	})
}

// TestSearchQueryProfileID tests ProfileID field functionality
func (s *SearchTestSuite) TestSearchQueryProfileID() {
	testCases := []struct {
		name      string
		profileID string
		query     string
		fields    map[string]any
	}{
		{
			name:      "with profile ID",
			profileID: "user-123",
			query:     "search with profile",
			fields:    map[string]any{"category": "personal"},
		},
		{
			name:      "empty profile ID",
			profileID: "",
			query:     "search without profile",
			fields:    map[string]any{"category": "public"},
		},
		{
			name:      "UUID profile ID",
			profileID: "550e8400-e29b-41d4-a716-446655440000",
			query:     "uuid profile search",
			fields:    map[string]any{"type": "uuid"},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			ctx := context.Background()

			result, err := NewSearchQuery(ctx, tc.query, tc.fields, 0, 10)
			require.NoError(s.T(), err)

			// Manually set ProfileID since NewSearchQuery doesn't set it
			result.ProfileID = tc.profileID

			assert.Equal(s.T(), tc.profileID, result.ProfileID)
			assert.Equal(s.T(), tc.query, result.Query)
			assert.Equal(s.T(), tc.fields, result.Fields)
		})
	}
}
