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

package frame

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/pitabwire/frame/version"
)

var ErrHealthCheckFailed = errors.New("health check failed")

// HealthResponse is the JSON structure returned by the health check endpoint.
type HealthResponse struct {
	Service    string              `json:"service"`
	Version    string              `json:"version"`
	Repository string              `json:"repository,omitempty"`
	Commit     string              `json:"commit,omitempty"`
	BuildDate  string              `json:"build_date,omitempty"`
	Uptime     string              `json:"uptime"`
	Status     string              `json:"status"`
	Checks     []HealthCheckResult `json:"checks"`
}

// HealthCheckResult holds the result of an individual health checker.
type HealthCheckResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func (s *Service) HealthCheckers() []Checker {
	return s.healthCheckers
}

// HandleHealth returns 200 with a JSON health report if all checks pass, 500 otherwise.
func (s *Service) HandleHealth(w http.ResponseWriter, _ *http.Request) {
	overall := "healthy"
	statusCode := http.StatusOK

	checks := make([]HealthCheckResult, 0, len(s.healthCheckers))
	for i, c := range s.healthCheckers {
		result := HealthCheckResult{
			Name:   checkerName(c, i),
			Status: "healthy",
		}
		if err := c.CheckHealth(); err != nil {
			result.Status = "unhealthy"
			result.Error = err.Error()
			overall = "unhealthy"
			statusCode = http.StatusInternalServerError
		}
		checks = append(checks, result)
	}

	resp := HealthResponse{
		Service:    s.Name(),
		Version:    s.Version(),
		Repository: version.Repository,
		Commit:     version.Commit,
		BuildDate:  version.Date,
		Uptime:     s.uptime(),
		Status:     overall,
		Checks:     checks,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(resp)
}

// HandleHealthByDefault returns the health report at root, 404 for other paths.
func (s *Service) HandleHealthByDefault(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "" || r.URL.Path == "/" {
		s.HandleHealth(w, r)
		return
	}

	http.NotFound(w, r)
}

func (s *Service) uptime() string {
	if s.startedAt.IsZero() {
		return "0s"
	}
	return time.Since(s.startedAt).Truncate(time.Second).String()
}

// checkerName returns a display name for a health checker. If the checker
// implements the NamedChecker interface its name is used, otherwise a
// positional fallback is generated.
func checkerName(c Checker, index int) string {
	if nc, ok := c.(NamedChecker); ok {
		return nc.Name()
	}
	return fmt.Sprintf("check-%d", index)
}

// WithHealthCheckPath Option checks that the system is up and running.
func WithHealthCheckPath(path string) Option {
	return func(_ context.Context, s *Service) {
		s.healthCheckPath = path
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

// NamedChecker is an optional interface that a Checker can implement to
// provide a human-readable name in health check responses.
type NamedChecker interface {
	Name() string
}

// CheckerFunc is an adapter type to allow the use of ordinary functions as
// health checks. If f is a function with the appropriate signature,
// CheckerFunc(f) is a Checker that calls f.
type CheckerFunc func() error

// CheckHealth calls f().
func (f CheckerFunc) CheckHealth() error {
	return f()
}
