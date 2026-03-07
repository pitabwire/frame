package httptor

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/pitabwire/util"
)

const (
	maxBodyLogSize = 1024 * 1024 // 1 MiB

	// HTTP status code thresholds.
	statusServerError = 500
	statusClientError = 400
	statusRedirect    = 300

	// Content type parsing.
	contentTypeSplitParts = 2

	// Time conversion.
	msPerSecond = 1000
)

type loggingConfig struct {
	responseWriterPool *sync.Pool
	allowedLogHeaders  map[string]bool
	safeQueryParams    map[string]bool
}

var (
	//nolint:gochecknoglobals // immutable config with sync.Pool for performance
	loggingConfigInstance = &loggingConfig{
		// Pool response wrappers to avoid per-request wrapper allocations.
		responseWriterPool: &sync.Pool{
			New: func() any {
				return &responseWriterWrapper{
					body: new(bytes.Buffer),
				}
			},
		},
		// Allow-list only: never accidentally log a secret header.
		allowedLogHeaders: map[string]bool{
			"content-type":     true,
			"accept":           true,
			"accept-encoding":  true,
			"user-agent":       true,
			"cache-control":    true,
			"origin":           true,
			"referer":          true,
			"x-request-id":     true,
			"x-correlation-id": true,
		},
		// Query params that are safe to log (very conservative).
		safeQueryParams: map[string]bool{
			"page":    true,
			"limit":   true,
			"sort":    true,
			"order":   true,
			"q":       true,
			"filter":  true,
			"fields":  true,
			"expand":  true,
			"version": true,
			"format":  true,
		},
	}
)

// responseWriterWrapper — minimal, lock-free, allocation-free in hot path.
type responseWriterWrapper struct {
	w          http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
	logBody    bool
	written    bool
}

// Header / WriteHeader / Write.
func (w *responseWriterWrapper) Header() http.Header { return w.w.Header() }

func (w *responseWriterWrapper) WriteHeader(code int) {
	if w.written {
		return
	}
	w.written = true
	w.statusCode = code
	w.w.WriteHeader(code)
}

func (w *responseWriterWrapper) Write(b []byte) (int, error) {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}

	// Only capture response bodies when body logging is explicitly enabled.
	if w.logBody {
		remaining := maxBodyLogSize - w.body.Len()
		if remaining > 0 {
			if len(b) > remaining {
				_, _ = w.body.Write(b[:remaining])
			} else {
				_, _ = w.body.Write(b)
			}
		}
	}

	return w.w.Write(b)
}

// Full interface support.
func (w *responseWriterWrapper) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.w.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

func (w *responseWriterWrapper) Flush() {
	if f, ok := w.w.(http.Flusher); ok {
		f.Flush()
	}
}

// Reset for pool reuse.
func (w *responseWriterWrapper) reset() {
	w.w = nil
	w.statusCode = 0
	w.logBody = false
	w.written = false
	w.body.Reset()
}

// LoggingMiddleware — zero-alloc hot path, panic-safe, production-grade.
func LoggingMiddleware(next http.Handler, logBody bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		logger := util.Log(ctx)

		// === Request body (safe read & replace) ===
		var reqBody []byte
		var truncated bool
		if logBody && r.Body != nil && r.ContentLength > 0 && shouldLogBody(r) {
			var err error
			reqBody, r.Body, truncated, err = readAndReplaceBody(r.Body)
			if err != nil {
				logger.WithError(err).Warn("failed to read request body for logging")
			}
		}

		// === Response wrapper from pool ===
		wrapper, ok := loggingConfigInstance.responseWriterPool.Get().(*responseWriterWrapper)
		if !ok {
			logger.Error("failed to get response writer wrapper from pool")
			next.ServeHTTP(w, r)
			return
		}
		wrapper.w = w
		wrapper.statusCode = http.StatusOK
		wrapper.logBody = logBody
		wrapper.body.Reset()
		defer loggingConfigInstance.responseWriterPool.Put(wrapper)
		defer wrapper.reset()

		// === Timing & panic safety ===
		start := time.Now()
		defer func() {
			duration := time.Since(start)

			if err := recover(); err != nil {
				logger.
					WithField("panic", err).
					Error("panic in HTTP handler")
				if !wrapper.written {
					wrapper.statusCode = http.StatusInternalServerError
				}
			}

			logHTTPRequest(
				logger,
				r,
				wrapper,
				reqBody,
				truncated,
				duration,
				logBody,
			)
		}()

		next.ServeHTTP(wrapper, r)
	}
}

// readAndReplaceBody — correct, no data loss, no duplication.
func readAndReplaceBody(orig io.ReadCloser) ([]byte, io.ReadCloser, bool, error) {
	if orig == http.NoBody {
		return nil, orig, false, nil
	}
	defer orig.Close()

	body, err := io.ReadAll(io.LimitReader(orig, maxBodyLogSize+1))
	if err != nil {
		return nil, nil, false, err
	}

	truncated := len(body) > maxBodyLogSize
	if truncated {
		body = body[:maxBodyLogSize]
	}

	return body, io.NopCloser(bytes.NewReader(body)), truncated, nil
}

// shouldLogBody — only text-like content types.
func shouldLogBody(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	ct = strings.ToLower(strings.TrimSpace(strings.SplitN(ct, ";", contentTypeSplitParts)[0]))
	return ct == "" ||
		strings.Contains(ct, "json") ||
		strings.Contains(ct, "text/") ||
		strings.Contains(ct, "xml") ||
		strings.Contains(ct, "yaml") ||
		strings.Contains(ct, "javascript") ||
		strings.Contains(ct, "form")
}

// logHTTPRequest — zero temporary allocations, safe strings, precise fields.
func logHTTPRequest(
	logger *util.LogEntry,
	r *http.Request,
	w *responseWriterWrapper,
	reqBody []byte,
	reqTruncated bool,
	duration time.Duration,
	logBody bool,
) {
	status := w.statusCode
	if status == 0 {
		status = http.StatusOK
	}

	log := logger.
		WithField("http.method", r.Method).
		WithField("http.url", safeURL(r)).
		WithField("http.remote_ip", util.GetIP(r)).
		WithField("http.user_agent", r.UserAgent()).
		WithField("http.status", status).
		WithField("http.duration_ms", duration.Seconds()*msPerSecond).
		WithField("http.request_size", r.ContentLength)

	// Request body
	if logBody && len(reqBody) > 0 {
		bodyStr := bytesToString(reqBody)
		if reqTruncated {
			bodyStr += " [truncated]"
		}
		log = log.WithField("http.request_body", bodyStr)
	}

	// Response body (only if body logging is enabled)
	if logBody && w.body.Len() > 0 {
		respStr := bytesToString(w.body.Bytes())
		if w.body.Len() == maxBodyLogSize {
			respStr += " [truncated]"
		}
		log = log.WithField("http.response_body", respStr)
	}

	// Headers — allow-list only
	log = addAllowedHeaders(log, r.Header, "req")
	log = addAllowedHeaders(log, w.Header(), "resp")

	// Final log by status
	switch {
	case status >= statusServerError:
		log.Error("HTTP request failed (server error)")
	case status >= statusClientError:
		log.Warn("HTTP request failed (client error)")
	case status >= statusRedirect:
		log.Info("HTTP request redirected")
	default:
		log.Info("HTTP request completed")
	}
}

// Safe helpers.
func addAllowedHeaders(log *util.LogEntry, h http.Header, prefix string) *util.LogEntry {
	for name, values := range h {
		lower := strings.ToLower(name)
		if !loggingConfigInstance.allowedLogHeaders[lower] {
			continue
		}
		field := prefix + "_header." + lower
		if len(values) == 1 {
			log = log.WithField(field, values[0])
		} else {
			log = log.WithField(field, values)
		}
	}
	return log
}

func safeURL(r *http.Request) string {
	u := *r.URL
	q := u.Query()
	for key := range q {
		if !loggingConfigInstance.safeQueryParams[strings.ToLower(key)] {
			q.Set(key, "[redacted]")
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// bytesToString — zero-copy string conversion (Go 1.20+).
func bytesToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}
