package frameworker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/util"
)

// Mock configuration for testing
type mockWorkerPoolConfig struct {
	cpuFactor      int
	capacity       int
	count          int
	expiryDuration time.Duration
}

func (m *mockWorkerPoolConfig) GetCPUFactor() int           { return m.cpuFactor }
func (m *mockWorkerPoolConfig) GetCapacity() int           { return m.capacity }
func (m *mockWorkerPoolConfig) GetCount() int              { return m.count }
func (m *mockWorkerPoolConfig) GetExpiryDuration() time.Duration { return m.expiryDuration }

// Test suite extending FrameBaseTestSuite
type FrameWorkerTestSuite struct {
	frametests.FrameBaseTestSuite
}

func TestFrameWorkerTestSuite(t *testing.T) {
	testSuite := &FrameWorkerTestSuite{}
	// Initialize the InitResourceFunc to satisfy the requirement
	testSuite.InitResourceFunc = func(ctx context.Context) []definition.TestResource {
		// Return empty slice since we're testing the worker manager directly
		return []definition.TestResource{}
	}
	suite.Run(t, testSuite)
}

// Test table-driven tests for worker functionality
func (suite *FrameWorkerTestSuite) TestWorkerManagerFunctionality() {
	tests := []struct {
		name string
		test func(ctx context.Context, suite *FrameWorkerTestSuite)
	}{
		{"JobResultImplementation", suite.testJobResultImplementation},
		{"JobResultCreation", suite.testJobResultCreation},
		{"JobImplBasicFunctionality", suite.testJobImplBasicFunctionality},
		{"JobImplResultPipe", suite.testJobImplResultPipe},
		{"JobImplConcurrency", suite.testJobImplConcurrency},
		{"WorkerManagerCreation", suite.testWorkerManagerCreation},
		{"WorkerManagerSubmitTask", suite.testWorkerManagerSubmitTask},
		{"WorkerManagerSubmitJob", suite.testWorkerManagerSubmitJob},
		{"WorkerManagerShutdown", suite.testWorkerManagerShutdown},
		{"WorkerPoolOptions", suite.testWorkerPoolOptions},
		{"ErrorHandling", suite.testErrorHandling},
		{"ConcurrentJobExecution", suite.testConcurrentJobExecution},
		{"JobRetryMechanism", suite.testJobRetryMechanism},
		{"PerformanceUnderLoad", suite.testPerformanceUnderLoad},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			ctx := context.Background()
			tt.test(ctx, suite)
		})
	}
}

func (suite *FrameWorkerTestSuite) testJobResultImplementation(ctx context.Context, _ *FrameWorkerTestSuite) {
	// Test successful result
	successResult := &jobResult[string]{item: "success", error: nil}
	suite.False(successResult.IsError())
	suite.NoError(successResult.Error())
	suite.Equal("success", successResult.Item())
	
	// Test error result
	testErr := errors.New("test error")
	errorResult := &jobResult[string]{item: "", error: testErr}
	suite.True(errorResult.IsError())
	suite.Equal(testErr, errorResult.Error())
	suite.Equal("", errorResult.Item())
	
	// Test with different types
	intResult := &jobResult[int]{item: 42, error: nil}
	suite.False(intResult.IsError())
	suite.Equal(42, intResult.Item())
	
	boolResult := &jobResult[bool]{item: true, error: nil}
	suite.False(boolResult.IsError())
	suite.True(boolResult.Item())
}

func (suite *FrameWorkerTestSuite) testJobResultCreation(ctx context.Context, _ *FrameWorkerTestSuite) {
	// Test Result function
	result := Result("test value")
	suite.False(result.IsError())
	suite.NoError(result.Error())
	suite.Equal("test value", result.Item())
	
	// Test ErrorResult function
	testErr := errors.New("test error")
	errorResult := ErrorResult[string](testErr)
	suite.True(errorResult.IsError())
	suite.Equal(testErr, errorResult.Error())
	suite.Equal("", errorResult.Item())
	
	// Test with different types
	intResult := Result(123)
	suite.Equal(123, intResult.Item())
	
	structResult := Result(struct{ Name string }{Name: "test"})
	suite.Equal("test", structResult.Item().Name)
}

func (suite *FrameWorkerTestSuite) testJobImplBasicFunctionality(ctx context.Context, _ *FrameWorkerTestSuite) {
	// Create a basic job
	processFunc := func(ctx context.Context, result JobResultPipe[string]) error {
		return result.WriteResult(ctx, "processed")
	}
	
	job := NewJobWithBufferAndRetry(processFunc, 5, 3)
	
	// Test basic properties
	suite.NotEmpty(job.ID()) // ID is auto-generated
	suite.Equal(3, job.Retries())
	suite.Equal(0, job.Runs())
	suite.True(job.CanRun())
	suite.Equal(5, job.ResultBufferSize())
	
	// Test runs increment
	job.IncreaseRuns()
	suite.Equal(1, job.Runs())
	
	job.IncreaseRuns()
	suite.Equal(2, job.Runs())
	
	// Test CanRun with max retries
	for i := 0; i < 2; i++ {
		job.IncreaseRuns()
	}
	suite.False(job.CanRun()) // Should be false when runs > retries
}

func (suite *FrameWorkerTestSuite) testJobImplResultPipe(ctx context.Context, _ *FrameWorkerTestSuite) {
	processFunc := func(ctx context.Context, result JobResultPipe[string]) error {
		return nil
	}
	
	job := NewJobWithBufferAndRetry(processFunc, 3, 1)
	
	// Test writing results
	err := job.WriteResult(ctx, "result1")
	suite.NoError(err)
	
	err = job.WriteResult(ctx, "result2")
	suite.NoError(err)
	
	// Test writing error
	testErr := errors.New("test error")
	err = job.WriteError(ctx, testErr)
	suite.NoError(err)
	
	// Test reading results
	result1, ok := job.ReadResult(ctx)
	suite.True(ok)
	suite.False(result1.IsError())
	suite.Equal("result1", result1.Item())
	
	result2, ok := job.ReadResult(ctx)
	suite.True(ok)
	suite.False(result2.IsError())
	suite.Equal("result2", result2.Item())
	
	errorResult, ok := job.ReadResult(ctx)
	suite.True(ok)
	suite.True(errorResult.IsError())
	suite.Equal(testErr, errorResult.Error())
	
	// Test result channel
	resultChan := job.ResultChan()
	suite.NotNil(resultChan)
	
	// Test close
	job.Close()
	
	// Writing after close should return error
	err = job.WriteResult(ctx, "after-close")
	suite.Equal(ErrWorkerPoolResultChannelIsClosed, err)
}

func (suite *FrameWorkerTestSuite) testJobImplConcurrency(ctx context.Context, _ *FrameWorkerTestSuite) {
	processFunc := func(ctx context.Context, result JobResultPipe[string]) error {
		return nil
	}
	
	job := NewJobWithBufferAndRetry(processFunc, 50, 100)
	
	// Test concurrent writes and reads
	var wg sync.WaitGroup
	numWriters := 10
	numReaders := 5
	itemsPerWriter := 5
	
	// Start writers
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			for j := 0; j < itemsPerWriter; j++ {
				value := fmt.Sprintf("writer-%d-item-%d", writerID, j)
				err := job.WriteResult(ctx, value)
				suite.NoError(err)
			}
		}(i)
	}
	
	// Start readers
	results := make([]string, 0, numWriters*itemsPerWriter)
	var resultsMutex sync.Mutex
	
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				result, ok := job.ReadResult(ctx)
				if !ok {
					return
				}
				if !result.IsError() {
					resultsMutex.Lock()
					results = append(results, result.Item())
					resultsMutex.Unlock()
				}
			}
		}()
	}
	
	// Wait for writers to complete
	time.Sleep(100 * time.Millisecond)
	job.Close()
	
	// Wait for all goroutines
	wg.Wait()
	
	// Verify we got all results
	suite.Equal(numWriters*itemsPerWriter, len(results))
	
	// Test concurrent runs increment
	job2 := NewJobWithBufferAndRetry(processFunc, 10, 1000)
	var runsWg sync.WaitGroup
	
	for i := 0; i < 100; i++ {
		runsWg.Add(1)
		go func() {
			defer runsWg.Done()
			job2.IncreaseRuns()
		}()
	}
	
	runsWg.Wait()
	suite.Equal(100, job2.Runs())
}

func (suite *FrameWorkerTestSuite) testWorkerManagerCreation(ctx context.Context, _ *FrameWorkerTestSuite) {
	logger := util.Log(ctx)
	
	// Test NewManager with valid options
	options := &WorkerPoolOptions{
		PoolCount:          1,
		SinglePoolCapacity: 10,
		Concurrency:        2,
		ExpiryDuration:     30 * time.Second,
		Nonblocking:        false,
		PreAlloc:           true,
		Logger:             logger,
		DisablePurge:       false,
	}
	
	manager, err := NewManager(options)
	suite.NoError(err)
	suite.NotNil(manager)
	suite.NotNil(manager.pool)
	suite.Equal(options, manager.options)
	suite.Equal(logger, manager.logger)
	
	// Test NewManagerWithDefaults
	config := &mockWorkerPoolConfig{
		cpuFactor:      2,
		capacity:       20,
		count:          3,
		expiryDuration: 60 * time.Second,
	}
	
	defaultManager, err := NewManagerWithDefaults(config, logger)
	suite.NoError(err)
	suite.NotNil(defaultManager)
	
	// Cleanup
	manager.Shutdown()
	defaultManager.Shutdown()
}

func (suite *FrameWorkerTestSuite) testWorkerManagerSubmitTask(ctx context.Context, _ *FrameWorkerTestSuite) {
	logger := util.Log(ctx)
	options := &WorkerPoolOptions{
		PoolCount:          1,
		SinglePoolCapacity: 10,
		Concurrency:        2,
		ExpiryDuration:     30 * time.Second,
		Logger:             logger,
	}
	
	manager, err := NewManager(options)
	suite.NoError(err)
	defer manager.Shutdown()
	
	// Test simple task submission
	var executed atomic.Bool
	task := func() {
		executed.Store(true)
	}
	
	err = manager.Submit(ctx, task)
	suite.NoError(err)
	
	// Wait for task execution
	time.Sleep(100 * time.Millisecond)
	suite.True(executed.Load())
	
	// Test multiple task submissions
	var counter atomic.Int64
	numTasks := 10
	
	for i := 0; i < numTasks; i++ {
		err = manager.Submit(ctx, func() {
			counter.Add(1)
		})
		suite.NoError(err)
	}
	
	// Wait for all tasks to complete
	time.Sleep(200 * time.Millisecond)
	suite.Equal(int64(numTasks), counter.Load())
}

func (suite *FrameWorkerTestSuite) testWorkerManagerSubmitJob(ctx context.Context, _ *FrameWorkerTestSuite) {
	logger := util.Log(ctx)
	options := &WorkerPoolOptions{
		PoolCount:          1,
		SinglePoolCapacity: 10,
		Concurrency:        2,
		ExpiryDuration:     30 * time.Second,
		Logger:             logger,
	}
	
	manager, err := NewManager(options)
	suite.NoError(err)
	defer manager.Shutdown()
	
	// Test job submission with any type
	processFunc := func(ctx context.Context, result JobResultPipe[any]) error {
		return result.WriteResult(ctx, "job completed")
	}
	
	job := NewJobWithBufferAndRetry(processFunc, 5, 1)
	
	err = manager.SubmitJob(ctx, job)
	suite.NoError(err)
	
	// Read the result
	result, ok := job.ReadResult(ctx)
	suite.True(ok)
	suite.False(result.IsError())
	suite.Equal("job completed", result.Item())
	
	job.Close()
}

func (suite *FrameWorkerTestSuite) testWorkerManagerShutdown(ctx context.Context, _ *FrameWorkerTestSuite) {
	logger := util.Log(ctx)
	options := &WorkerPoolOptions{
		PoolCount:          1,
		SinglePoolCapacity: 10,
		Concurrency:        2,
		ExpiryDuration:     30 * time.Second,
		Logger:             logger,
	}
	
	manager, err := NewManager(options)
	suite.NoError(err)
	
	// Submit some tasks before shutdown
	var counter atomic.Int64
	for i := 0; i < 5; i++ {
		err = manager.Submit(ctx, func() {
			time.Sleep(10 * time.Millisecond)
			counter.Add(1)
		})
		suite.NoError(err)
	}
	
	// Wait a bit for tasks to start
	time.Sleep(50 * time.Millisecond)
	
	// Shutdown should wait for tasks to complete
	manager.Shutdown()
	
	// Verify tasks completed
	suite.Equal(int64(5), counter.Load())
}

func (suite *FrameWorkerTestSuite) testWorkerPoolOptions(ctx context.Context, _ *FrameWorkerTestSuite) {
	logger := util.Log(ctx)
	
	// Test various option combinations
	testCases := []struct {
		name    string
		options *WorkerPoolOptions
	}{
		{
			name: "SinglePool",
			options: &WorkerPoolOptions{
				PoolCount:          1,
				SinglePoolCapacity: 20,
				Concurrency:        4,
				ExpiryDuration:     60 * time.Second,
				Nonblocking:        false,
				PreAlloc:           true,
				Logger:             logger,
			},
		},
		{
			name: "MultiPool",
			options: &WorkerPoolOptions{
				PoolCount:          3,
				SinglePoolCapacity: 10,
				Concurrency:        2,
				ExpiryDuration:     30 * time.Second,
				Nonblocking:        true,
				PreAlloc:           false,
				Logger:             logger,
			},
		},
	}
	
	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			manager, err := NewManager(tc.options)
			suite.NoError(err)
			suite.NotNil(manager)
			
			// Test basic functionality
			var executed atomic.Bool
			err = manager.Submit(ctx, func() {
				executed.Store(true)
			})
			suite.NoError(err)
			
			time.Sleep(100 * time.Millisecond)
			suite.True(executed.Load())
			
			manager.Shutdown()
		})
	}
}

func (suite *FrameWorkerTestSuite) testErrorHandling(ctx context.Context, _ *FrameWorkerTestSuite) {
	logger := util.Log(ctx)
	
	// Test manager with nil pool (should fail)
	invalidOptions := &WorkerPoolOptions{
		PoolCount:          0,
		SinglePoolCapacity: 0,
		Logger:             logger,
	}
	
	_, err := NewManager(invalidOptions)
	suite.Error(err)
	
	// Test job with error in process function
	options := &WorkerPoolOptions{
		PoolCount:          1,
		SinglePoolCapacity: 10,
		Concurrency:        2,
		ExpiryDuration:     30 * time.Second,
		Logger:             logger,
	}
	
	manager, err := NewManager(options)
	suite.NoError(err)
	defer manager.Shutdown()
	
	testErr := errors.New("process error")
	processFunc := func(ctx context.Context, result JobResultPipe[any]) error {
		return result.WriteError(ctx, testErr)
	}
	
	job := NewJobWithBufferAndRetry(processFunc, 5, 1)
	
	err = manager.SubmitJob(ctx, job)
	suite.NoError(err)
	
	// Read the error result
	result, ok := job.ReadResult(ctx)
	suite.True(ok)
	suite.True(result.IsError())
	suite.Equal(testErr, result.Error())
	
	job.Close()
}

func (suite *FrameWorkerTestSuite) testConcurrentJobExecution(ctx context.Context, _ *FrameWorkerTestSuite) {
	logger := util.Log(ctx)
	options := &WorkerPoolOptions{
		PoolCount:          2,
		SinglePoolCapacity: 20,
		Concurrency:        5,
		ExpiryDuration:     30 * time.Second,
		Logger:             logger,
	}
	
	manager, err := NewManager(options)
	suite.NoError(err)
	defer manager.Shutdown()
	
	numJobs := 10
	jobs := make([]Job[any], numJobs)
	
	// Create and submit multiple jobs
	for i := 0; i < numJobs; i++ {
		jobID := i
		processFunc := func(ctx context.Context, result JobResultPipe[any]) error {
			// Simulate some work
			time.Sleep(10 * time.Millisecond)
			return result.WriteResult(ctx, jobID*2)
		}
		
		jobs[i] = NewJobWithBufferAndRetry(processFunc, 5, 1)
		err = manager.SubmitJob(ctx, jobs[i])
		suite.NoError(err)
	}
	
	// Collect results
	results := make([]any, numJobs)
	for i := 0; i < numJobs; i++ {
		result, ok := jobs[i].ReadResult(ctx)
		suite.True(ok)
		suite.False(result.IsError())
		results[i] = result.Item()
		jobs[i].Close()
	}
	
	// Verify all jobs completed
	suite.Len(results, numJobs)
}

func (suite *FrameWorkerTestSuite) testJobRetryMechanism(ctx context.Context, _ *FrameWorkerTestSuite) {
	// Test job retry logic
	processFunc := func(ctx context.Context, result JobResultPipe[string]) error {
		return nil
	}
	job := NewJobWithBufferAndRetry(processFunc, 5, 3)
	
	// Initially should be able to run
	suite.True(job.CanRun())
	suite.Equal(0, job.Runs())
	
	// Increase runs and check CanRun
	for i := 1; i <= 3; i++ {
		job.IncreaseRuns()
		suite.Equal(i, job.Runs())
		suite.True(job.CanRun()) // Should still be able to run within retry limit
	}
	
	// Exceed retry limit
	job.IncreaseRuns()
	suite.Equal(4, job.Runs())
	suite.False(job.CanRun()) // Should not be able to run anymore
}

func (suite *FrameWorkerTestSuite) testPerformanceUnderLoad(ctx context.Context, _ *FrameWorkerTestSuite) {
	logger := util.Log(ctx)
	options := &WorkerPoolOptions{
		PoolCount:          4,
		SinglePoolCapacity: 50,
		Concurrency:        10,
		ExpiryDuration:     30 * time.Second,
		Logger:             logger,
	}
	
	manager, err := NewManager(options)
	suite.NoError(err)
	defer manager.Shutdown()
	
	// Submit many tasks quickly
	numTasks := 1000
	var counter atomic.Int64
	start := time.Now()
	
	for i := 0; i < numTasks; i++ {
		err = manager.Submit(ctx, func() {
			counter.Add(1)
		})
		suite.NoError(err)
	}
	
	// Wait for all tasks to complete
	for counter.Load() < int64(numTasks) {
		time.Sleep(1 * time.Millisecond)
	}
	
	duration := time.Since(start)
	suite.Equal(int64(numTasks), counter.Load())
	
	// Performance should be reasonable (less than 5 seconds for 1000 simple tasks)
	suite.Less(duration, 5*time.Second)
}

// Benchmark tests for performance validation
func BenchmarkJobResult_IsError(b *testing.B) {
	result := Result("test")
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = result.IsError()
	}
}

func BenchmarkJobImpl_IncreaseRuns(b *testing.B) {
	processFunc := func(ctx context.Context, result JobResultPipe[string]) error {
		return nil
	}
	job := NewJobWithBufferAndRetry(processFunc, 10, 1000000)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		job.IncreaseRuns()
	}
}

func BenchmarkWorkerManager_Submit(b *testing.B) {
	logger := util.Log(context.Background())
	options := &WorkerPoolOptions{
		PoolCount:          2,
		SinglePoolCapacity: 100,
		Concurrency:        10,
		ExpiryDuration:     30 * time.Second,
		Logger:             logger,
	}
	
	manager, _ := NewManager(options)
	defer manager.Shutdown()
	
	ctx := context.Background()
	task := func() {}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = manager.Submit(ctx, task)
	}
}
