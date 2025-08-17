package framehealth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

const healthWatchIntervalSeconds = 5

var ErrHealthCheckFailed = errors.New("health check failed")

// CheckHealth calls f().
func (f CheckerFunc) CheckHealth() error {
	return f()
}

// Manager implements the HealthManager interface
type Manager struct {
	checkers []Checker
	mutex    sync.RWMutex
}

// NewManager creates a new health manager
func NewManager() *Manager {
	return &Manager{
		checkers: make([]Checker, 0),
	}
}

// AddChecker adds a health checker to the manager
func (m *Manager) AddChecker(checker Checker) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.checkers = append(m.checkers, checker)
}

// RemoveChecker removes a health checker from the manager
func (m *Manager) RemoveChecker(checker Checker) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	for i, c := range m.checkers {
		if c == checker {
			m.checkers = append(m.checkers[:i], m.checkers[i+1:]...)
			return true
		}
	}
	return false
}

// GetCheckers returns all registered health checkers
func (m *Manager) GetCheckers() []Checker {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	// Return a copy to prevent external modification
	checkers := make([]Checker, len(m.checkers))
	copy(checkers, m.checkers)
	return checkers
}

// CheckHealth runs all health checks and returns the overall health status
func (m *Manager) CheckHealth() error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	for _, c := range m.checkers {
		if err := c.CheckHealth(); err != nil {
			return err
		}
	}
	return nil
}

// HandleHealth handles HTTP health check requests
// Returns 200 if it is healthy, 500 otherwise.
func (m *Manager) HandleHealth(w http.ResponseWriter, _ *http.Request) {
	if err := m.CheckHealth(); err != nil {
		writeUnhealthy(w)
		return
	}
	writeHealthy(w)
}

// HandleHealthByDefault handles HTTP health check requests with default routing
// Returns 200 if it is healthy, 500 when there is an err or 404 otherwise.
func (m *Manager) HandleHealthByDefault(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "" || r.URL.Path == "/" {
		m.HandleHealth(w, r)
		return
	}
	http.NotFound(w, r)
}

// GetGrpcHealthServer returns a gRPC health server implementation
func (m *Manager) GetGrpcHealthServer() grpc_health_v1.HealthServer {
	return &grpcHealthServer{manager: m}
}

func writeHeaders(statusLen string, w http.ResponseWriter) {
	w.Header().Set("Content-Length", statusLen)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

func writeUnhealthy(w http.ResponseWriter) {
	const (
		status    = "unhealthy"
		statusLen = "9"
	)

	writeHeaders(statusLen, w)
	w.WriteHeader(http.StatusInternalServerError)
	_, err := io.WriteString(w, status)
	if err != nil {
		return
	}
}

func writeHealthy(w http.ResponseWriter) {
	const (
		status    = "ok"
		statusLen = "2"
	)

	writeHeaders(statusLen, w)
	w.WriteHeader(http.StatusOK)
	_, err := io.WriteString(w, status)
	if err != nil {
		return
	}
}

// grpcHealthServer implements the gRPC health server
type grpcHealthServer struct {
	grpc_health_v1.UnimplementedHealthServer
	manager *Manager
}

func (ghs *grpcHealthServer) Check(
	_ context.Context,
	_ *grpc_health_v1.HealthCheckRequest,
) (*grpc_health_v1.HealthCheckResponse, error) {
	if err := ghs.manager.CheckHealth(); err != nil {
		return &grpc_health_v1.HealthCheckResponse{
			Status: grpc_health_v1.HealthCheckResponse_NOT_SERVING,
		}, err
	}

	return &grpc_health_v1.HealthCheckResponse{
		Status: grpc_health_v1.HealthCheckResponse_SERVING,
	}, nil
}

func (ghs *grpcHealthServer) Watch(
	_ *grpc_health_v1.HealthCheckRequest,
	stream grpc_health_v1.Health_WatchServer,
) error {
	var lastSentStatus grpc_health_v1.HealthCheckResponse_ServingStatus = -1
	for {
		select {
		// Status updated. Sends the up-to-date status to the client.
		case <-time.After(healthWatchIntervalSeconds * time.Second):
			servingStatus := grpc_health_v1.HealthCheckResponse_SERVING
			if err := ghs.manager.CheckHealth(); err != nil {
				servingStatus = grpc_health_v1.HealthCheckResponse_NOT_SERVING
			}

			if lastSentStatus == servingStatus {
				continue
			}
			lastSentStatus = servingStatus
			err := stream.Send(&grpc_health_v1.HealthCheckResponse{Status: servingStatus})
			if err != nil {
				return status.Error(codes.Canceled, "Stream has ended.")
			}

		// Context done. Removes the update channel from the updates map.
		case <-stream.Context().Done():
			return status.Error(codes.Canceled, "Stream has ended.")
		}
	}
}
