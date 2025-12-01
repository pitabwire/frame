package http

import (
	"bufio"
	"bytes"
	"context"
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
	if w.body != nil && w.body.Len() < maxBodyLogSize {
		// Only buffer up to the max log size.
		remainingSpace := maxBodyLogSize - w.body.Len()
		if len(b) > remainingSpace {
			w.body.Write(b[:remainingSpace])
		} else {
			w.body.Write(b)
		}
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
func LoggingMiddleware(next http.Handler, logBody bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		logger := util.Log(ctx)

		requestBody := readRequestBody(r, logBody)
		wrapped := wrapResponseWriter(w)

		start := time.Now()
		next.ServeHTTP(wrapped, r)
		duration := time.Since(start)

		logHTTPRequest(logger, r, wrapped, requestBody, duration)
	})
}

// ContextLoggingMiddleware propagates logger in main context into HTTP context.
func ContextLoggingMiddleware(mainCtx context.Context, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := util.Log(mainCtx)
		ctx := util.ContextWithLogger(r.Context(), logger)

		// Replace the request with the merged context
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}

// readRequestBody reads and returns the request body, restoring it for further processing.
func readRequestBody(r *http.Request, logBody bool) []byte {
	var requestBody []byte
	if logBody && r.Body != nil {
		// Read only up to the max log size to avoid loading large bodies into memory.
		lr := io.LimitReader(r.Body, maxBodyLogSize)
		requestBody, _ = io.ReadAll(lr)
		// Re-construct the body with what was read plus the rest of the original stream.
		r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(requestBody), r.Body))
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
	// Use a map for efficient, case-insensitive lookups after canonicalization.
	sensitiveHeaders := map[string]struct{}{
		"Authorization": {},
		"Cookie":        {},
		"Set-Cookie":    {},
		"X-Api-Key":     {},
		"X-Auth-Token":  {},
		"X-Csrf-Token":  {},
		"X-Session-Id":  {},
	}
	_, ok := sensitiveHeaders[http.CanonicalHeaderKey(name)]
	return ok
}
