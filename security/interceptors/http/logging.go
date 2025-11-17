package http

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/pitabwire/util"
)

const (
	maxBodyLogSize = 1024 // Max body size to log (1KB)
	clientError    = 400
	serverError    = 500
)

// responseWriterWrapper wraps http.ResponseWriter to capture response status and body.
type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
}

// WriteHeader captures the status code.
func (w *responseWriterWrapper) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

// Write captures the response body.
func (w *responseWriterWrapper) Write(b []byte) (int, error) {
	if w.body != nil {
		w.body.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

// Hijack implements http.Hijacker if the underlying ResponseWriter supports it.
func (w *responseWriterWrapper) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// LoggingMiddleware creates an HTTP middleware that logs requests and responses when tracing is enabled.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		logger := util.Log(ctx)

		requestBody := readRequestBody(r)
		wrapped := wrapResponseWriter(w)

		start := time.Now()
		next.ServeHTTP(wrapped, r)
		duration := time.Since(start)

		logHTTPRequest(logger, r, wrapped, requestBody, duration)
	})
}

// readRequestBody reads and returns the request body, restoring it for further processing.
func readRequestBody(r *http.Request) []byte {
	var requestBody []byte
	if r.Body != nil {
		requestBody, _ = io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewBuffer(requestBody))
	}
	return requestBody
}

// wrapResponseWriter creates a wrapped response writer to capture status and body.
func wrapResponseWriter(w http.ResponseWriter) *responseWriterWrapper {
	return &responseWriterWrapper{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		body:           &bytes.Buffer{},
	}
}

// logHTTPRequest logs the HTTP request and response details.
func logHTTPRequest(
	logger *util.LogEntry,
	r *http.Request,
	wrapped *responseWriterWrapper,
	requestBody []byte,
	duration time.Duration,
) {
	logEntry := logger.WithFields(map[string]any{
		"method":         r.Method,
		"path":           r.URL.Path,
		"query":          r.URL.RawQuery,
		"remote_addr":    r.RemoteAddr,
		"user_agent":     r.UserAgent(),
		"status_code":    wrapped.statusCode,
		"duration_ms":    duration.Milliseconds(),
		"content_length": r.ContentLength})

	logEntry = addRequestHeaders(logEntry, r.Header)
	logEntry = addRequestBody(logEntry, requestBody)
	logEntry = addResponseHeaders(logEntry, wrapped.Header())
	logEntry = addResponseBody(logEntry, wrapped.body)

	logByStatusCode(logEntry, wrapped.statusCode)
}

// addRequestHeaders adds non-sensitive request headers to the log entry.
func addRequestHeaders(logEntry *util.LogEntry, headers http.Header) *util.LogEntry {
	for name, values := range headers {
		if !isSensitiveHeader(name) {
			logEntry = logEntry.WithField("req_header_"+name, values)
		}
	}
	return logEntry
}

// addRequestBody adds the request body to the log entry if within size limits.
func addRequestBody(logEntry *util.LogEntry, requestBody []byte) *util.LogEntry {
	if len(requestBody) > 0 && len(requestBody) < maxBodyLogSize {
		logEntry = logEntry.WithField("request_body", string(requestBody))
	}
	return logEntry
}

// addResponseHeaders adds non-sensitive response headers to the log entry.
func addResponseHeaders(logEntry *util.LogEntry, headers http.Header) *util.LogEntry {
	for name, values := range headers {
		if !isSensitiveHeader(name) {
			logEntry = logEntry.WithField("resp_header_"+name, values)
		}
	}
	return logEntry
}

// addResponseBody adds the response body to the log entry if within size limits.
func addResponseBody(logEntry *util.LogEntry, body *bytes.Buffer) *util.LogEntry {
	if body.Len() > 0 && body.Len() < maxBodyLogSize {
		logEntry = logEntry.WithField("response_body", body.String())
	}
	return logEntry
}

// logByStatusCode logs the message at the appropriate level based on status code.
func logByStatusCode(logEntry *util.LogEntry, statusCode int) {
	switch {
	case statusCode >= serverError:
		logEntry.Error("HTTP request completed with server error")
	case statusCode >= clientError:
		logEntry.Warn("HTTP request completed with client error")
	default:
		logEntry.Info("HTTP request completed successfully")
	}
}

// isSensitiveHeader checks if a header contains sensitive information.
func isSensitiveHeader(name string) bool {
	sensitiveHeaders := []string{
		"authorization",
		"cookie",
		"set-cookie",
		"x-api-key",
		"x-auth-token",
		"x-csrf-token",
		"x-session-id",
	}

	for _, sensitive := range sensitiveHeaders {
		if name == sensitive {
			return true
		}
	}
	return false
}
