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
//
// Kubernetes health endpoints follow:
// https://kubernetes.io/docs/reference/using-api/health-checks/
//
//   /livez   — liveness: restart only if the process is non-recoverable
//   /readyz  — readiness: ready to accept traffic
//   /healthz — deprecated alias of readiness (kept for compatibility)

package frame

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/pitabwire/frame/v2/version"
)

// Standard Kubernetes health check paths.
// See https://kubernetes.io/docs/reference/using-api/health-checks/
const (
	DefaultLivenessPath  = "/livez"
	DefaultReadinessPath = "/readyz"
	DefaultHealthzPath   = "/healthz"

	probeStatusHealthy   = "healthy"
	probeStatusUnhealthy = "unhealthy"
)

var (
	ErrHealthCheckFailed = errors.New("health check failed")
	ErrNotReady          = errors.New("service is not ready")
	ErrStartupIncomplete = errors.New("startup not completed")
	ErrTerminating       = errors.New("service is terminating")
)

// HealthResponse is the JSON structure returned by health endpoints.
// Machines (kubelet probes) should rely on the HTTP status code only;
// the body is for human operators.
type HealthResponse struct {
	Service    string              `json:"service"`
	Version    string              `json:"version"`
	Repository string              `json:"repository,omitempty"`
	Commit     string              `json:"commit,omitempty"`
	BuildDate  string              `json:"build_date,omitempty"`
	Uptime     string              `json:"uptime"`
	Probe      string              `json:"probe"` // livez | readyz | healthz
	Status     string              `json:"status"`
	Checks     []HealthCheckResult `json:"checks"`
}

// HealthCheckResult holds the result of an individual health checker.
type HealthCheckResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// Checker wraps the CheckHealth method.
//
// CheckHealth returns nil if the resource is healthy, or a non-nil
// error if the resource is not healthy. CheckHealth must be safe to
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

// HealthCheckers returns readiness checkers (dependency checks used by /readyz
// and the deprecated /healthz endpoint).
func (s *Service) HealthCheckers() []Checker {
	return s.healthCheckers
}

// LivenessCheckers returns process-level liveness checkers used by /livez.
func (s *Service) LivenessCheckers() []Checker {
	return s.livenessCheckers
}

// AddHealthCheck registers a readiness/dependency check.
// These run on /readyz and /healthz. Do not use for conditions that should
// restart the process — use AddLivenessCheck for that rare case.
//
// Typical use: database ping, cache connectivity, critical dependency health.
func (s *Service) AddHealthCheck(checker Checker) {
	if s.healthCheckers == nil {
		s.healthCheckers = []Checker{}
	}
	s.healthCheckers = append(s.healthCheckers, checker)
}

// AddLivenessCheck registers a process-level liveness check used by /livez.
// Prefer leaving liveness empty: an HTTP response already proves the process
// is responsive. Only add checks for non-recoverable process faults (e.g.
// deadlock detectors). Never check external dependencies here — a flaky
// dependency would cause restart loops.
func (s *Service) AddLivenessCheck(checker Checker) {
	if s.livenessCheckers == nil {
		s.livenessCheckers = []Checker{}
	}
	s.livenessCheckers = append(s.livenessCheckers, checker)
}

// WithHealthCheckPath sets an additional path that serves the healthz
// (readiness-compatible) report. The standard Kubernetes paths /livez,
// /readyz, and /healthz are always registered.
func WithHealthCheckPath(path string) Option {
	return func(_ context.Context, s *Service) {
		s.healthCheckPath = path
	}
}

// WithLivenessPath overrides the default /livez path.
func WithLivenessPath(path string) Option {
	return func(_ context.Context, s *Service) {
		s.livenessPath = path
	}
}

// WithReadinessPath overrides the default /readyz path.
func WithReadinessPath(path string) Option {
	return func(_ context.Context, s *Service) {
		s.readinessPath = path
	}
}

// HandleLivez is the Kubernetes liveness probe handler (/livez).
//
// Returns 200 when the process is alive and should not be restarted.
// Does not evaluate readiness/dependency checkers. Remains healthy during
// graceful shutdown so the kubelet does not SIGKILL the pod while traffic
// drains. Fails only when optional liveness checkers report a non-recoverable
// process fault.
//
// See https://kubernetes.io/docs/reference/using-api/health-checks/
func (s *Service) HandleLivez(w http.ResponseWriter, _ *http.Request) {
	s.writeProbeResponse(w, "livez", s.evaluateLiveness())
}

// HandleReadyz is the Kubernetes readiness probe handler (/readyz).
//
// Returns 200 when the service is ready to accept traffic. Fails (503) when:
//   - startup hooks have not completed
//   - the service is terminating (graceful shutdown / SIGTERM)
//   - any registered readiness checker fails
//
// See https://kubernetes.io/docs/reference/using-api/health-checks/
func (s *Service) HandleReadyz(w http.ResponseWriter, _ *http.Request) {
	s.writeProbeResponse(w, "readyz", s.evaluateReadiness())
}

// HandleHealthz is the deprecated Kubernetes healthz handler (/healthz).
// Behaviour matches readiness (/readyz). Prefer /livez and /readyz for new
// deployments. Kept for backward compatibility with existing probes and tools.
//
// See https://kubernetes.io/docs/reference/using-api/health-checks/
func (s *Service) HandleHealthz(w http.ResponseWriter, _ *http.Request) {
	s.writeProbeResponse(w, "healthz", s.evaluateReadiness())
}

// HandleHealth is an alias of HandleHealthz retained for backward compatibility.
func (s *Service) HandleHealth(w http.ResponseWriter, r *http.Request) {
	s.HandleHealthz(w, r)
}

// HandleHealthByDefault returns the healthz report at root, 404 for other paths.
func (s *Service) HandleHealthByDefault(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "" || r.URL.Path == "/" {
		s.HandleHealthz(w, r)
		return
	}
	http.NotFound(w, r)
}

type probeResult struct {
	status string
	checks []HealthCheckResult
	err    error
}

func (s *Service) evaluateLiveness() probeResult {
	// Liveness must stay green during termination so in-flight work can finish.
	// A process that can still run this handler is, by default, live.
	checks, err := runCheckers(s.livenessCheckers)
	if err != nil {
		return probeResult{status: probeStatusUnhealthy, checks: checks, err: err}
	}
	return probeResult{status: probeStatusHealthy, checks: checks}
}

func (s *Service) evaluateReadiness() probeResult {
	var checks []HealthCheckResult

	if !s.isStartupCompleted() {
		checks = append(checks, HealthCheckResult{
			Name:   "startup",
			Status: probeStatusUnhealthy,
			Error:  ErrStartupIncomplete.Error(),
		})
		return probeResult{status: probeStatusUnhealthy, checks: checks, err: ErrStartupIncomplete}
	}

	if s.isTerminating() {
		checks = append(checks, HealthCheckResult{
			Name:   "shutdown",
			Status: probeStatusUnhealthy,
			Error:  ErrTerminating.Error(),
		})
		return probeResult{status: probeStatusUnhealthy, checks: checks, err: ErrTerminating}
	}

	depChecks, err := runCheckers(s.healthCheckers)
	checks = append(checks, depChecks...)
	if err != nil {
		return probeResult{status: probeStatusUnhealthy, checks: checks, err: err}
	}
	return probeResult{status: probeStatusHealthy, checks: checks}
}

func runCheckers(checkers []Checker) ([]HealthCheckResult, error) {
	results := make([]HealthCheckResult, 0, len(checkers))
	var firstErr error
	for i, c := range checkers {
		result := HealthCheckResult{
			Name:   checkerName(c, i),
			Status: probeStatusHealthy,
		}
		if err := c.CheckHealth(); err != nil {
			result.Status = probeStatusUnhealthy
			result.Error = err.Error()
			if firstErr == nil {
				firstErr = err
			}
		}
		results = append(results, result)
	}
	return results, firstErr
}

func (s *Service) writeProbeResponse(w http.ResponseWriter, probe string, result probeResult) {
	statusCode := http.StatusOK
	if result.err != nil {
		// 503 is the conventional probe failure code: the process is up but
		// this probe condition is not satisfied. Kubelet treats any non-2xx
		// as failure; 503 is more accurate than 500 for readiness drain.
		statusCode = http.StatusServiceUnavailable
	}

	resp := HealthResponse{
		Service:    s.Name(),
		Version:    s.Version(),
		Repository: version.Repository,
		Commit:     version.Commit,
		BuildDate:  version.Date,
		Uptime:     s.uptime(),
		Probe:      probe,
		Status:     result.status,
		Checks:     result.checks,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(resp)
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

func (s *Service) isStartupCompleted() bool {
	s.startupRegistrations.Lock()
	defer s.startupRegistrations.Unlock()
	return s.startupCompleted
}

func (s *Service) isTerminating() bool {
	return s.terminating.Load()
}

// markTerminating flips readiness off immediately so traffic is drained while
// the process remains live for in-flight request completion.
func (s *Service) markTerminating() {
	s.terminating.Store(true)
}

func (s *Service) registerHealthEndpoints(mux *http.ServeMux) {
	livePath := s.livenessPath
	if livePath == "" {
		livePath = DefaultLivenessPath
	}
	readyPath := s.readinessPath
	if readyPath == "" {
		readyPath = DefaultReadinessPath
	}
	healthzPath := s.healthCheckPath
	if healthzPath == "" || (healthzPath == "/" && s.handler != nil) {
		healthzPath = DefaultHealthzPath
	}
	// Persist normalized path so tests and debug introspection see the effective value.
	s.healthCheckPath = healthzPath
	if s.livenessPath == "" {
		s.livenessPath = livePath
	}
	if s.readinessPath == "" {
		s.readinessPath = readyPath
	}

	registered := map[string]struct{}{}
	register := func(path string, h http.HandlerFunc) {
		if path == "" {
			return
		}
		if _, ok := registered[path]; ok {
			return
		}
		mux.HandleFunc(path, h)
		registered[path] = struct{}{}
	}

	register(livePath, s.HandleLivez)
	register(readyPath, s.HandleReadyz)
	register(healthzPath, s.HandleHealthz)

	// Always expose the standard Kubernetes names even when paths are customized.
	register(DefaultLivenessPath, s.HandleLivez)
	register(DefaultReadinessPath, s.HandleReadyz)
	register(DefaultHealthzPath, s.HandleHealthz)
}
