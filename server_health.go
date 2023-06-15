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
	"io"
	"net/http"
)

func (s *Service) HealthCheckers() []Checker {
	return s.healthCheckers
}

// HandleHealth returns 200 if it is healthy, 500 otherwise.
func (s *Service) HandleHealth(w http.ResponseWriter, _ *http.Request) {
	for _, c := range s.healthCheckers {
		if err := c.CheckHealth(); err != nil {
			writeUnhealthy(w)
			return
		}
	}
	writeHealthy(w)
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

// LivelinessPath Option checks that the system is up and running
func LivelinessPath(path string) Option {
	return func(s *Service) {
		s.livelinessPath = path
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
