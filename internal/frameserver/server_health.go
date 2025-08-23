// Copyright 2018 The Go Cloud Development Kit Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// Picked from : "gocloud.dev/server/health"

package frameserver

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/pitabwire/frame/internal/common"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

const healthWatchIntervalSeconds = 5

var ErrHealthCheckFailed = errors.New("health check failed")

func HealthCheckers(s common.Service) []Checker {
	if healthModule, ok := s.GetModule(common.ModuleTypeHealth).(common.HealthModule); ok {
		checkers := healthModule.HealthCheckers()
		result := make([]Checker, len(checkers))
		for i, checker := range checkers {
			if c, ok := checker.(Checker); ok {
				result[i] = c
			}
		}
		return result
	}
	return []Checker{}
}

// HandleHealth returns 200 if it is healthy, 500 otherwise.
func HandleHealth(s common.Service, w http.ResponseWriter, _ *http.Request) {
	healthCheckers := HealthCheckers(s)
	
	for _, c := range healthCheckers {
		if err := c.CheckHealth(); err != nil {
			writeUnhealthy(w)
			return
		}
	}
	writeHealthy(w)
}

// HandleHealthByDefault returns 200 if it is healthy, 500 when there is an err or 404 otherwise.
func HandleHealthByDefault(s common.Service, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "" || r.URL.Path == "/" {
		HandleHealth(s, w, r)
		return
	}

	http.NotFound(w, r)
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

// WithHealthCheckPath Option checks that the system is up and running.
func WithHealthCheckPath(path string) common.Option {
	return func(_ context.Context, s common.Service) {
		if healthModule, ok := s.GetModule(common.ModuleTypeHealth).(common.HealthModule); ok {
			// Update the existing HealthModule with the new health check path
			// Note: Cannot directly assign to interface - this needs to be refactored
			// TODO: Implement proper HealthModule update mechanism
			_ = healthModule // Use healthModule to avoid unused variable error
			_ = path         // Use path to avoid unused variable error
		}
	}
}

// Checker wraps the CheckHealth method.
//
// CheckHealth returns nil if the resource is healthy, or a non-nil
// error if the resource is not healthy.  CheckHealth must be safe to
// call from multiple goroutines.
type Checker interface {
	CheckHealth() error
}

// CheckerFunc is an adapter type to allow the use of ordinary functions as
// health checks. If f is a function with the appropriate signature,
// CheckerFunc(f) is a Checker that calls f.
type CheckerFunc func() error

// CheckHealth calls f().
func (f CheckerFunc) CheckHealth() error {
	return f()
}

type grpcHealthServer struct {
	grpc_health_v1.UnimplementedHealthServer
	service common.Service
}

func (ghs *grpcHealthServer) Check(
	_ context.Context,
	_ *grpc_health_v1.HealthCheckRequest,
) (*grpc_health_v1.HealthCheckResponse, error) {
	healthCheckers := []Checker{}
	if healthModule, ok := ghs.service.GetModule(common.ModuleTypeHealth).(common.HealthModule); ok {
		checkers := healthModule.HealthCheckers()
		healthCheckers = make([]Checker, len(checkers))
		for i, checker := range checkers {
			if c, ok := checker.(Checker); ok {
				healthCheckers[i] = c
			}
		}
	}
	
	for _, c := range healthCheckers {
		if err := c.CheckHealth(); err != nil {
			return &grpc_health_v1.HealthCheckResponse{
				Status: grpc_health_v1.HealthCheckResponse_NOT_SERVING,
			}, err
		}
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
			healthCheckers := []Checker{}
			if healthModule, ok := ghs.service.GetModule(common.ModuleTypeHealth).(common.HealthModule); ok {
				checkers := healthModule.HealthCheckers()
				healthCheckers = make([]Checker, len(checkers))
				for i, checker := range checkers {
					if c, ok := checker.(Checker); ok {
						healthCheckers[i] = c
					}
				}
			}
			
			for _, c := range healthCheckers {
				if err := c.CheckHealth(); err != nil {
					servingStatus = grpc_health_v1.HealthCheckResponse_NOT_SERVING
					break
				}
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

func NewGrpcHealthServer(service common.Service) grpc_health_v1.HealthServer {
	return &grpcHealthServer{
		service: service,
	}
}
