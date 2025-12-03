package http

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/pitabwire/util"
)

const (
	maxBodyLogSize = 1024 // Max body size to log (1KB)
	clientError    = 400
	serverError    = 500
)

var (
	// Pool for reusing responseWriterWrapper instances.
	//nolint:gochecknoglobals // Global pool is idiomatic for sync.Pool usage in middleware
	responseWriterPool = sync.Pool{
		New: func() interface{} {
			return &responseWriterWrapper{
				statusCode: http.StatusOK,
				body:       &bytes.Buffer{},
			}
		},
	}

	// Pre-defined sensitive headers map for O(1) lookups.
	//nolint:gochecknoglobals // Global map is appropriate for static sensitive header list
	sensitiveHeaders = map[string]struct{}{
		"Authorization": {},
		"Cookie":        {},
		"Set-Cookie":    {},
		"X-Api-Key":     {},
		"X-Auth-Token":  {},
		"X-Csrf-Token":  {},
		"X-Session-Id":  {},
	}
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

		// Ensure cleanup happens even if panic occurs
		defer releaseResponseWriter(wrapped)

		start := time.Now()
		next.ServeHTTP(wrapped, r)
		duration := time.Since(start)

		logHTTPRequest(logger, r, wrapped, requestBody, duration, logBody)
	})
}

// readRequestBody reads and returns the request body, restoring it for further processing.
func readRequestBody(r *http.Request, logBody bool) []byte {
	if !logBody || r.Body == nil {
		return nil
	}

	// Read only up to the max log size to avoid loading large bodies into memory.
	lr := io.LimitReader(r.Body, maxBodyLogSize)
	requestBody, _ := io.ReadAll(lr)
	// Re-construct the body with what was read plus the rest of the original stream.
	r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(requestBody), r.Body))
	return requestBody
}

// wrapResponseWriter creates a wrapped response writer to capture status and body.
func wrapResponseWriter(w http.ResponseWriter) *responseWriterWrapper {
	wrapped, ok := responseWriterPool.Get().(*responseWriterWrapper)
	if !ok {
		wrapped = &responseWriterWrapper{
			statusCode: http.StatusOK,
			body:       &bytes.Buffer{},
		}
	}
	wrapped.ResponseWriter = w
	wrapped.statusCode = http.StatusOK
	wrapped.body.Reset() // Clear any previous content
	return wrapped
}

// releaseResponseWriter returns the wrapper to the pool.
func releaseResponseWriter(w *responseWriterWrapper) {
	w.ResponseWriter = nil
	responseWriterPool.Put(w)
}

// logHTTPRequest logs the HTTP request and response details.
func logHTTPRequest(
	logger *util.LogEntry,
	r *http.Request,
	wrapped *responseWriterWrapper,
	requestBody []byte,
	duration time.Duration,
	logBody bool,
) {
	// Pre-allocate map with known capacity to reduce allocations
	const typicalFieldCount = 8
	fields := make(map[string]any, typicalFieldCount) // Estimate typical field count
	fields["method"] = r.Method
	fields["path"] = r.URL.Path
	fields["query"] = r.URL.RawQuery
	fields["remote_addr"] = r.RemoteAddr
	fields["user_agent"] = r.UserAgent()
	fields["status_code"] = wrapped.statusCode
	fields["duration_ms"] = duration.Milliseconds()
	fields["content_length"] = r.ContentLength

	logEntry := logger.WithFields(fields)

	// Only add body if it exists and within limits
	if logBody && len(requestBody) > 0 && len(requestBody) < maxBodyLogSize {
		logEntry = logEntry.WithField("request_body", string(requestBody))
	}

	if wrapped.body != nil && wrapped.body.Len() > 0 && wrapped.body.Len() < maxBodyLogSize {
		logEntry = logEntry.WithField("response_body", wrapped.body.String())
	}

	logEntry = addRequestHeaders(logEntry, r.Header)
	logEntry = addResponseHeaders(logEntry, wrapped.Header())

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

// addResponseHeaders adds non-sensitive response headers to the log entry.
func addResponseHeaders(logEntry *util.LogEntry, headers http.Header) *util.LogEntry {
	for name, values := range headers {
		if !isSensitiveHeader(name) {
			logEntry = logEntry.WithField("resp_header_"+name, values)
		}
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
	_, ok := sensitiveHeaders[http.CanonicalHeaderKey(name)]
	return ok
}
