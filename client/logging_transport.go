package client

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pitabwire/util"
)

// Copyright 2023-2024 Ant Investor Ltd
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

const (
	defaultMaxBodySize = 1024 // Max body size to log (1KB)
)

// LoggingTransportOption configures the logging HTTP transport.
type LoggingTransportOption func(*loggingTransport)

// loggingTransport is an HTTP transport that logs requests and responses.
type loggingTransport struct {
	transport    http.RoundTripper
	logRequests  bool
	logResponses bool
	logHeaders   bool
	logBody      bool
	maxBodySize  int64
}

// NewLoggingTransport creates a new logging HTTP transport.
// By default, it logs requests and responses but not headers or body for security.
func NewLoggingTransport(transport http.RoundTripper, opts ...LoggingTransportOption) http.RoundTripper {
	if transport == nil {
		transport = http.DefaultTransport
	}

	t := &loggingTransport{
		transport:    transport,
		logRequests:  true,
		logResponses: true,
		logHeaders:   false,              // Don't log headers by default for security
		logBody:      false,              // Don't log body by default for security/size
		maxBodySize:  defaultMaxBodySize, // Max body size to log (1KB)
	}

	for _, opt := range opts {
		opt(t)
	}

	return t
}

// WithTransportLogRequests enables or disables request logging.
func WithTransportLogRequests(enabled bool) LoggingTransportOption {
	return func(t *loggingTransport) {
		t.logRequests = enabled
	}
}

// WithTransportLogResponses enables or disables response logging.
func WithTransportLogResponses(enabled bool) LoggingTransportOption {
	return func(t *loggingTransport) {
		t.logResponses = enabled
	}
}

// WithTransportLogHeaders enables or disables header logging.
// Note: Be careful when enabling this as headers may contain sensitive information.
func WithTransportLogHeaders(enabled bool) LoggingTransportOption {
	return func(t *loggingTransport) {
		t.logHeaders = enabled
	}
}

// WithTransportLogBody enables or disables body logging.
// Note: Be careful when enabling this as bodies may contain sensitive information or be large.
func WithTransportLogBody(enabled bool) LoggingTransportOption {
	return func(t *loggingTransport) {
		t.logBody = enabled
	}
}

// WithTransportMaxBodySize sets the maximum body size to log.
func WithTransportMaxBodySize(size int64) LoggingTransportOption {
	return func(t *loggingTransport) {
		t.maxBodySize = size
	}
}

// RoundTrip implements http.RoundTripper.
func (t *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	ctx := req.Context()

	// Log the request
	if t.logRequests {
		t.logRequest(ctx, req)
	}

	// Execute the request
	resp, err := t.transport.RoundTrip(req)

	// Log the response
	if t.logResponses {
		duration := time.Since(start)
		t.logResponse(ctx, resp, err, duration)
	}

	return resp, err
}

func (t *loggingTransport) logRequest(ctx context.Context, req *http.Request) {
	if !t.logRequests {
		return
	}

	logger := util.Log(ctx).WithFields(map[string]any{
		"method": req.Method,
		"url":    req.URL.String(),
		"host":   req.Host,
	})

	if t.logHeaders {
		headers := make(map[string]string)
		for name, values := range req.Header {
			if len(values) > 0 {
				headers[name] = strings.Join(values, " , ")
			}
		}
		logger = logger.WithField("headers", headers)
	}

	if t.logBody && req.Body != nil {
		// Read the body to log it
		bodyBytes, err := io.ReadAll(io.LimitReader(req.Body, t.maxBodySize))
		if err == nil && len(bodyBytes) > 0 {
			logger = logger.WithField("body", string(bodyBytes))
			// Restore the body for the actual request
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}
	}

	logger.Info("HTTP request sent")
}

func (t *loggingTransport) logResponse(ctx context.Context, resp *http.Response, err error, duration time.Duration) {
	if !t.logResponses {
		return
	}

	logger := util.Log(ctx).WithFields(map[string]any{
		"duration": duration.String(),
	})

	if err != nil {
		logger.WithError(err).Error("HTTP request failed")
		return
	}

	if resp != nil {
		logger = t.logResponseDetails(logger, resp)
	}

	logger.Info("HTTP response received")
}

func (t *loggingTransport) logResponseDetails(logger *util.LogEntry, resp *http.Response) *util.LogEntry {
	logger = logger.WithFields(map[string]any{
		"status":     resp.StatusCode,
		"statusText": http.StatusText(resp.StatusCode),
	})

	if t.logHeaders {
		logger = t.logResponseHeaders(logger, resp.Header)
	}

	if t.logBody && resp.Body != nil {
		logger = t.logResponseBody(logger, &resp.Body)
	}
	return logger
}

func (t *loggingTransport) logResponseHeaders(logger *util.LogEntry, headers http.Header) *util.LogEntry {
	headerMap := make(map[string]string)
	for name, values := range headers {
		if len(values) > 0 {
			headerMap[name] = values[0]
		}
	}
	return logger.WithField("headers", headerMap)
}

func (t *loggingTransport) logResponseBody(logger *util.LogEntry, body *io.ReadCloser) *util.LogEntry {
	var bodyBytes []byte
	bodyBytes, readErr := io.ReadAll(io.LimitReader(*body, t.maxBodySize))
	if readErr == nil && len(bodyBytes) > 0 {
		logger = logger.WithField("body", string(bodyBytes))
		// Restore the body for the caller
		*body = io.NopCloser(bytes.NewReader(bodyBytes))
	}
	return logger
}

// WrapClient wraps an existing HTTP client with logging transport.
func WrapClient(client *http.Client, opts ...LoggingTransportOption) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}

	transport := NewLoggingTransport(client.Transport, opts...)
	newClient := *client
	newClient.Transport = transport
	return &newClient
}
