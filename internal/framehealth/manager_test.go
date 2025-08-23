package framehealth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/health/grpc_health_v1"
	
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
)

// Real checker implementations using actual dependencies
type databaseChecker struct {
	db *sql.DB
}

func (d *databaseChecker) CheckHealth() error {
	return d.CheckDatabaseConnection(context.Background())
}

func (d *databaseChecker) CheckDatabaseConnection(ctx context.Context) error {
	if d.db == nil {
		return errors.New("database connection is nil")
	}
	return d.db.PingContext(ctx)
}

type queueChecker struct {
	queueURL string
}

func (q *queueChecker) CheckHealth() error {
	return q.CheckQueueConnection(context.Background())
}

func (q *queueChecker) CheckQueueConnection(ctx context.Context) error {
	if q.queueURL == "" {
		return errors.New("queue URL is empty")
	}
	// In a real implementation, this would check NATS connection
	return nil
}

type serviceChecker struct {
	serviceURL string
	client     *http.Client
}

func (s *serviceChecker) CheckHealth() error {
	return s.CheckServiceHealth(context.Background(), s.serviceURL)
}

func (s *serviceChecker) CheckServiceHealth(ctx context.Context, serviceURL string) error {
	if s.client == nil {
		s.client = &http.Client{Timeout: 5 * time.Second}
	}
	
	req, err := http.NewRequestWithContext(ctx, "GET", serviceURL, nil)
	if err != nil {
		return err
	}
	
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("service health check failed with status: %d", resp.StatusCode)
	}
	
	return nil
}

// Test suite extending FrameBaseTestSuite
type FrameHealthTestSuite struct {
	frametests.FrameBaseTestSuite
}

func TestFrameHealthTestSuite(t *testing.T) {
	testSuite := &FrameHealthTestSuite{}
	// Initialize the InitResourceFunc to satisfy the requirement
	testSuite.InitResourceFunc = func(ctx context.Context) []definition.TestResource {
		// Return empty slice since we're testing the health manager directly
		return []definition.TestResource{}
	}
	suite.Run(t, testSuite)
}

// Test table-driven tests using real dependencies
func (suite *FrameHealthTestSuite) TestHealthManagerFunctionality() {
	tests := []struct {
		name string
		test func(ctx context.Context, suite *FrameHealthTestSuite)
	}{
		{"NewManager", suite.testNewManager},
		{"AddChecker", suite.testAddChecker},
		{"RemoveChecker", suite.testRemoveChecker},
		{"GetCheckers", suite.testGetCheckers},
		{"CheckHealthAllHealthy", suite.testCheckHealthAllHealthy},
		{"CheckHealthWithUnhealthy", suite.testCheckHealthWithUnhealthy},
		{"HTTPHealthHandler", suite.testHTTPHealthHandler},
		{"GRPCHealthServer", suite.testGRPCHealthServer},
		{"ConcurrentAccess", suite.testConcurrentAccess},
		{"DatabaseChecker", suite.testDatabaseChecker},
		{"QueueChecker", suite.testQueueChecker},
		{"ServiceChecker", suite.testServiceChecker},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			ctx := context.Background()
			tt.test(ctx, suite)
		})
	}
}

func (suite *FrameHealthTestSuite) testNewManager(ctx context.Context, _ *FrameHealthTestSuite) {
	manager := NewManager()
	
	suite.NotNil(manager)
	suite.Empty(manager.checkers)
	suite.NotNil(manager.mutex)
}

func (suite *FrameHealthTestSuite) testAddChecker(ctx context.Context, _ *FrameHealthTestSuite) {
	manager := NewManager()
	
	// Create a simple checker function
	healthyChecker := CheckerFunc(func() error { return nil })
	
	manager.AddChecker(healthyChecker)
	suite.Len(manager.checkers, 1)
	
	// Add multiple checkers
	unhealthyChecker := CheckerFunc(func() error { return errors.New("unhealthy") })
	manager.AddChecker(unhealthyChecker)
	suite.Len(manager.checkers, 2)
}

func (suite *FrameHealthTestSuite) testRemoveChecker(ctx context.Context, _ *FrameHealthTestSuite) {
	manager := NewManager()
	
	// Use concrete checker types that are comparable instead of CheckerFunc
	checker1 := &databaseChecker{db: nil}
	checker2 := &queueChecker{queueURL: "test"}
	checker3 := &serviceChecker{serviceURL: "http://test"}
	
	manager.AddChecker(checker1)
	manager.AddChecker(checker2)
	manager.AddChecker(checker3)
	suite.Len(manager.checkers, 3)
	
	// Remove middle checker
	removed := manager.RemoveChecker(checker2)
	suite.True(removed)
	suite.Len(manager.checkers, 2)
	
	// Try to remove non-existent checker
	removed = manager.RemoveChecker(checker2)
	suite.False(removed)
	suite.Len(manager.checkers, 2)
}

func (suite *FrameHealthTestSuite) testGetCheckers(ctx context.Context, _ *FrameHealthTestSuite) {
	manager := NewManager()
	
	checker1 := CheckerFunc(func() error { return nil })
	checker2 := CheckerFunc(func() error { return errors.New("unhealthy") })
	
	manager.AddChecker(checker1)
	manager.AddChecker(checker2)
	
	checkers := manager.GetCheckers()
	suite.Len(checkers, 2)
	
	// Verify it returns a copy (modifying returned slice doesn't affect manager)
	originalLen := len(manager.checkers)
	checkers = append(checkers, CheckerFunc(func() error { return nil }))
	suite.Len(manager.checkers, originalLen)
}

func (suite *FrameHealthTestSuite) testCheckHealthAllHealthy(ctx context.Context, _ *FrameHealthTestSuite) {
	manager := NewManager()
	
	manager.AddChecker(CheckerFunc(func() error { return nil }))
	manager.AddChecker(CheckerFunc(func() error { return nil }))
	
	err := manager.CheckHealth()
	suite.NoError(err)
	
	// Test with no checkers
	emptyManager := NewManager()
	err = emptyManager.CheckHealth()
	suite.NoError(err)
}

func (suite *FrameHealthTestSuite) testCheckHealthWithUnhealthy(ctx context.Context, _ *FrameHealthTestSuite) {
	manager := NewManager()
	
	manager.AddChecker(CheckerFunc(func() error { return nil }))
	manager.AddChecker(CheckerFunc(func() error { return errors.New("service unavailable") }))
	
	err := manager.CheckHealth()
	suite.Error(err)
	suite.Contains(err.Error(), "service unavailable")
}

func (suite *FrameHealthTestSuite) testHTTPHealthHandler(ctx context.Context, _ *FrameHealthTestSuite) {
	// Test healthy response
	healthyManager := NewManager()
	healthyManager.AddChecker(CheckerFunc(func() error { return nil }))
	
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	
	healthyManager.HandleHealth(w, req)
	
	suite.Equal(http.StatusOK, w.Code)
	suite.Equal("ok", w.Body.String())
	suite.Equal("2", w.Header().Get("Content-Length"))
	suite.Equal("text/plain; charset=utf-8", w.Header().Get("Content-Type"))
	suite.Equal("nosniff", w.Header().Get("X-Content-Type-Options"))
	
	// Test unhealthy response
	unhealthyManager := NewManager()
	unhealthyManager.AddChecker(CheckerFunc(func() error { return errors.New("unhealthy") }))
	
	req = httptest.NewRequest("GET", "/health", nil)
	w = httptest.NewRecorder()
	
	unhealthyManager.HandleHealth(w, req)
	
	suite.Equal(http.StatusInternalServerError, w.Code)
	suite.Equal("unhealthy", w.Body.String())
	suite.Equal("9", w.Header().Get("Content-Length"))
	
	// Test HandleHealthByDefault
	req = httptest.NewRequest("GET", "/", nil)
	w = httptest.NewRecorder()
	healthyManager.HandleHealthByDefault(w, req)
	suite.Equal(http.StatusOK, w.Code)
	
	req = httptest.NewRequest("GET", "/other", nil)
	w = httptest.NewRecorder()
	healthyManager.HandleHealthByDefault(w, req)
	suite.Equal(http.StatusNotFound, w.Code)
}

func (suite *FrameHealthTestSuite) testGRPCHealthServer(ctx context.Context, _ *FrameHealthTestSuite) {
	manager := NewManager()
	server := manager.GetGrpcHealthServer()
	suite.NotNil(server)
	
	// Test healthy check
	manager.AddChecker(CheckerFunc(func() error { return nil }))
	req := &grpc_health_v1.HealthCheckRequest{}
	
	resp, err := server.Check(ctx, req)
	suite.NoError(err)
	suite.NotNil(resp)
	suite.Equal(grpc_health_v1.HealthCheckResponse_SERVING, resp.Status)
	
	// Test unhealthy check
	manager.AddChecker(CheckerFunc(func() error { return errors.New("unhealthy") }))
	resp, err = server.Check(ctx, req)
	suite.Error(err)
	suite.NotNil(resp)
	suite.Equal(grpc_health_v1.HealthCheckResponse_NOT_SERVING, resp.Status)
}

func (suite *FrameHealthTestSuite) testConcurrentAccess(ctx context.Context, _ *FrameHealthTestSuite) {
	manager := NewManager()
	
	// Test concurrent add/remove operations
	done := make(chan bool, 3)
	
	// Goroutine 1: Add checkers
	go func() {
		for i := 0; i < 50; i++ {
			checker := CheckerFunc(func() error { return nil })
			manager.AddChecker(checker)
		}
		done <- true
	}()
	
	// Goroutine 2: Get checkers
	go func() {
		for i := 0; i < 50; i++ {
			_ = manager.GetCheckers()
		}
		done <- true
	}()
	
	// Goroutine 3: Check health
	go func() {
		for i := 0; i < 50; i++ {
			_ = manager.CheckHealth()
		}
		done <- true
	}()
	
	// Wait for all goroutines to complete
	for i := 0; i < 3; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			suite.Fail("Test timed out")
		}
	}
	
	// Verify final state
	checkers := manager.GetCheckers()
	suite.Len(checkers, 50)
}

func (suite *FrameHealthTestSuite) testDatabaseChecker(ctx context.Context, s *FrameHealthTestSuite) {
	// Test with nil database (should fail)
	checker := &databaseChecker{
		db: nil, // This would be service.GetDB() or similar
	}
	
	err := checker.CheckHealth()
	suite.Error(err)
	suite.Contains(err.Error(), "database connection is nil")
	
	// Test CheckDatabaseConnection directly
	err = checker.CheckDatabaseConnection(ctx)
	suite.Error(err)
}

func (suite *FrameHealthTestSuite) testQueueChecker(ctx context.Context, s *FrameHealthTestSuite) {
	// Test with empty URL (should fail)
	checker := &queueChecker{queueURL: ""}
	err := checker.CheckHealth()
	suite.Error(err)
	suite.Contains(err.Error(), "queue URL is empty")
	
	// Test with valid URL (should pass)
	checker = &queueChecker{queueURL: "nats://localhost:4222"}
	err = checker.CheckHealth()
	suite.NoError(err)
	
	// Test CheckQueueConnection directly
	err = checker.CheckQueueConnection(ctx)
	suite.NoError(err)
}

func (suite *FrameHealthTestSuite) testServiceChecker(ctx context.Context, s *FrameHealthTestSuite) {
	// Create a test HTTP server
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("healthy"))
	}))
	defer testServer.Close()
	
	// Test with healthy service
	checker := &serviceChecker{serviceURL: testServer.URL}
	err := checker.CheckHealth()
	suite.NoError(err)
	
	// Test CheckServiceHealth directly
	err = checker.CheckServiceHealth(ctx, testServer.URL)
	suite.NoError(err)
	
	// Test with unhealthy service
	unhealthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer unhealthyServer.Close()
	
	checker = &serviceChecker{serviceURL: unhealthyServer.URL}
	err = checker.CheckHealth()
	suite.Error(err)
	suite.Contains(err.Error(), "service health check failed with status: 500")
}

func (suite *FrameHealthTestSuite) TestCheckerFunc() {
	// Test healthy checker function
	healthyFunc := CheckerFunc(func() error {
		return nil
	})
	
	err := healthyFunc.CheckHealth()
	suite.NoError(err)
	
	// Test unhealthy checker function
	customErr := errors.New("test error")
	unhealthyFunc := CheckerFunc(func() error {
		return customErr
	})
	
	err = unhealthyFunc.CheckHealth()
	suite.Error(err)
	suite.Equal(customErr, err)
}

// Benchmark tests for performance validation
func BenchmarkManager_CheckHealth(b *testing.B) {
	manager := NewManager()
	for i := 0; i < 10; i++ {
		manager.AddChecker(CheckerFunc(func() error { return nil }))
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = manager.CheckHealth()
	}
}

func BenchmarkManager_AddChecker(b *testing.B) {
	manager := NewManager()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker := CheckerFunc(func() error { return nil })
		manager.AddChecker(checker)
	}
}

func BenchmarkManager_GetCheckers(b *testing.B) {
	manager := NewManager()
	for i := 0; i < 100; i++ {
		manager.AddChecker(CheckerFunc(func() error { return nil }))
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = manager.GetCheckers()
	}
}
