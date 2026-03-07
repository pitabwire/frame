package client

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/pitabwire/util"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/pitabwire/frame/config"
)

const (
	defaultHTTPTimeoutSeconds     = 30
	defaultHTTPIdleTimeoutSeconds = 90

	defaultMaxRetryAttempts = 3
	defaultMinRetryDuration = 100 * time.Millisecond
)

var (
	ErrWorkloadAPITargetPathRequiresTrustDomain = errors.New(
		"workload API target path requires a trust domain",
	)
	ErrWorkloadAPIH2CIncompatible = errors.New(
		"workload API mTLS cannot be combined with h2c",
	)
	ErrWorkloadAPITransportUnsupported = errors.New(
		"workload API mTLS requires *http.Transport",
	)
)

type RetryPolicy struct {
	MaxAttempts int
	Backoff     func(attempt int) time.Duration
}

// HTTPOption configures HTTP client behavior.
// It can be used to configure timeout, transport, and other HTTP client settings.
type HTTPOption func(*httpConfig)

// httpConfig holds HTTP client configuration.
type httpConfig struct {
	timeout       time.Duration
	transport     http.RoundTripper
	jar           http.CookieJar
	checkRedirect func(req *http.Request, via []*http.Request) error
	idleTimeout   time.Duration
	enableH2C     bool

	cliCredCfg *clientcredentials.Config

	traceRequests       bool
	traceRequestHeaders bool

	retryPolicy            *RetryPolicy
	retryPolicyConfigured  bool
	workloadAPIRequested   bool
	workloadAPITrustDomain string
	workloadAPITargetID    string
	workloadAPITargetPath  string
}

func (hc *httpConfig) process(ctx context.Context, opts ...HTTPOption) {
	for _, opt := range opts {
		opt(hc)
	}

	if hc.workloadAPITrustDomain == "" {
		hc.workloadAPITrustDomain = resolveWorkloadAPITrustDomain(ctx)
	}

	if hc.workloadAPITargetID == "" &&
		hc.workloadAPITargetPath != "" &&
		hc.workloadAPITrustDomain != "" {
		hc.workloadAPITargetID = buildWorkloadAPITargetID(
			hc.workloadAPITrustDomain,
			hc.workloadAPITargetPath,
		)
	}

	if hc.retryPolicy == nil {
		hc.retryPolicy = &RetryPolicy{
			MaxAttempts: defaultMaxRetryAttempts,
			Backoff: func(attempt int) time.Duration {
				return time.Duration(attempt*attempt) * defaultMinRetryDuration
			},
		}
	}
}

// WithHTTPTimeout sets the request timeout.
func WithHTTPTimeout(timeout time.Duration) HTTPOption {
	return func(c *httpConfig) {
		c.timeout = timeout
	}
}

// WithHTTPTransport sets the HTTP transport.
func WithHTTPTransport(transport http.RoundTripper) HTTPOption {
	return func(c *httpConfig) {
		c.transport = transport
	}
}

// WithHTTPCookieJar sets the cookie jar.
func WithHTTPCookieJar(jar http.CookieJar) HTTPOption {
	return func(c *httpConfig) {
		c.jar = jar
	}
}

// WithHTTPCheckRedirect sets the redirect policy.
func WithHTTPCheckRedirect(checkRedirect func(req *http.Request, via []*http.Request) error) HTTPOption {
	return func(c *httpConfig) {
		c.checkRedirect = checkRedirect
	}
}

// WithHTTPIdleTimeout sets the idle timeout.
func WithHTTPIdleTimeout(timeout time.Duration) HTTPOption {
	return func(c *httpConfig) {
		c.idleTimeout = timeout
	}
}

// WithHTTPEnableH2C sets the enable h2c option to active.
func WithHTTPEnableH2C() HTTPOption {
	return func(c *httpConfig) {
		c.enableH2C = true
	}
}

// WithHTTPClientCredentials the client credentials the client can utilize.
func WithHTTPClientCredentials(cfg *clientcredentials.Config) HTTPOption {
	return func(c *httpConfig) {
		c.cliCredCfg = cfg
	}
}

// WithHTTPTraceRequests enables or disables request logging.
func WithHTTPTraceRequests() HTTPOption {
	return func(c *httpConfig) {
		c.traceRequests = true
	}
}

// WithHTTPTraceRequestHeaders enables or disables header logging.
func WithHTTPTraceRequestHeaders() HTTPOption {
	return func(c *httpConfig) {
		c.traceRequestHeaders = true
	}
}

// WithHTTPRetryPolicy setts required retry policy.
func WithHTTPRetryPolicy(retryPolicy *RetryPolicy) HTTPOption {
	return func(c *httpConfig) {
		c.retryPolicyConfigured = true
		c.retryPolicy = retryPolicy
	}
}

// WithHTTPWorkloadAPITargetID configures SPIFFE mTLS for outbound calls and
// authorizes the exact upstream SPIFFE ID.
func WithHTTPWorkloadAPITargetID(targetID string) HTTPOption {
	return func(c *httpConfig) {
		c.workloadAPIRequested = true
		c.workloadAPITargetID = strings.TrimSpace(targetID)
	}
}

// WithHTTPWorkloadAPITargetPath configures SPIFFE mTLS for an upstream path
// within the configured trust domain.
func WithHTTPWorkloadAPITargetPath(targetPath string) HTTPOption {
	return func(c *httpConfig) {
		c.workloadAPIRequested = true
		c.workloadAPITargetPath = strings.TrimSpace(targetPath)
	}
}

// WithHTTPWorkloadAPITrustDomain configures trust-domain-wide SPIFFE mTLS.
// Prefer exact target IDs or target paths for service-to-service traffic.
func WithHTTPWorkloadAPITrustDomain(trustDomain string) HTTPOption {
	return func(c *httpConfig) {
		c.workloadAPIRequested = true
		c.workloadAPITrustDomain = strings.TrimSpace(trustDomain)
	}
}

// NewHTTPClient creates a new HTTP client with the provided options.
func NewHTTPClient(ctx context.Context, opts ...HTTPOption) *http.Client {
	client, _ := newHTTPClient(ctx, opts...)
	return client
}

func newHTTPClient(ctx context.Context, opts ...HTTPOption) (*http.Client, error) {
	cfg := &httpConfig{
		timeout:     time.Duration(defaultHTTPTimeoutSeconds) * time.Second,
		idleTimeout: time.Duration(defaultHTTPIdleTimeoutSeconds) * time.Second,
		transport:   http.DefaultTransport,
	}

	cfg.process(ctx, opts...)

	base, closeIdleFn, err := prepareBaseTransport(ctx, cfg)
	if err != nil {
		return newErrorHTTPClient(cfg, err), err
	}

	if _, ok := base.(*otelhttp.Transport); !ok {
		base = otelhttp.NewTransport(base)
	}

	base = newResilientTransport(base, cfg.retryPolicy)

	if cfg.traceRequests {
		base = NewLoggingTransport(base,
			WithTransportLogRequests(true),
			WithTransportLogResponses(true),
			WithTransportLogHeaders(cfg.traceRequestHeaders),
			WithTransportLogBody(false))
	}

	if closeIdleFn != nil {
		base = closeIdleTransport{
			inner:       base,
			closeIdleFn: closeIdleFn,
		}
	}

	client := &http.Client{
		Transport:     base,
		Timeout:       cfg.timeout,
		Jar:           cfg.jar,
		CheckRedirect: cfg.checkRedirect,
	}

	if cfg.cliCredCfg != nil {
		oauth2Ctx := context.WithValue(ctx, oauth2.HTTPClient, client)
		client = cfg.cliCredCfg.Client(oauth2Ctx)
	}

	return client, nil
}

func newErrorHTTPClient(cfg *httpConfig, err error) *http.Client {
	if cfg == nil {
		cfg = &httpConfig{}
	}

	return &http.Client{
		Transport:     errorTransport{err: err},
		Timeout:       cfg.timeout,
		Jar:           cfg.jar,
		CheckRedirect: cfg.checkRedirect,
	}
}

func resolveWorkloadAPITrustDomain(ctx context.Context) string {
	cfg := config.FromContext[any](ctx)
	workloadCfg, ok := cfg.(config.ConfigurationWorkloadAPI)
	if !ok {
		return ""
	}

	return strings.TrimSpace(workloadCfg.GetTrustedDomain())
}

func prepareBaseTransport(ctx context.Context, cfg *httpConfig) (http.RoundTripper, func(), error) {
	base := cfg.transport
	transport, ok := base.(*http.Transport)
	if !ok {
		if cfg.workloadAPIRequested {
			return nil, nil, ErrWorkloadAPITransportUnsupported
		}
		if cfg.enableH2C {
			util.Log(ctx).Warn("ignoring h2c configuration for non-http transport")
		}
		return base, nil, nil
	}

	transport = transport.Clone()

	if cfg.workloadAPIRequested {
		if cfg.enableH2C {
			return nil, nil, ErrWorkloadAPIH2CIncompatible
		}

		tlsConfig, source, err := newWorkloadAPIMTLSClientConfig(ctx, cfg)
		if err != nil {
			return nil, nil, err
		}

		transport.TLSClientConfig = tlsConfig
		managed := newManagedTransport(transport, source)
		return managed, managed.CloseIdleConnections, nil
	}

	if cfg.enableH2C {
		protocols := new(http.Protocols)
		protocols.SetUnencryptedHTTP2(true)
		transport.Protocols = protocols
	}

	if cfg.idleTimeout > 0 {
		transport.IdleConnTimeout = cfg.idleTimeout
	}

	return transport, nil, nil
}

func newWorkloadAPIMTLSClientConfig(
	ctx context.Context,
	cfg *httpConfig,
) (*tls.Config, *workloadapi.X509Source, error) {
	if cfg == nil {
		return nil, nil, errors.New("http config is required")
	}

	authorizer, err := newWorkloadAPIAuthorizer(cfg)
	if err != nil {
		return nil, nil, err
	}

	source, err := workloadapi.NewX509Source(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("create x509 source: %w", err)
	}

	tlsCfg := tlsconfig.MTLSClientConfig(source, source, authorizer)
	return tlsCfg, source, nil
}

func newWorkloadAPIAuthorizer(cfg *httpConfig) (tlsconfig.Authorizer, error) {
	if cfg == nil {
		return nil, errors.New("http config is required")
	}

	if cfg.workloadAPITargetID != "" {
		serverID, err := spiffeid.FromString(strings.TrimSpace(cfg.workloadAPITargetID))
		if err != nil {
			return nil, err
		}
		return tlsconfig.AuthorizeID(serverID), nil
	}

	if cfg.workloadAPITargetPath != "" {
		if strings.TrimSpace(cfg.workloadAPITrustDomain) == "" {
			return nil, ErrWorkloadAPITargetPathRequiresTrustDomain
		}

		td, err := spiffeid.TrustDomainFromString(strings.TrimSpace(cfg.workloadAPITrustDomain))
		if err != nil {
			return nil, err
		}

		serverID, err := spiffeid.FromPath(td, strings.TrimSpace(cfg.workloadAPITargetPath))
		if err != nil {
			return nil, err
		}

		return tlsconfig.AuthorizeID(serverID), nil
	}

	if strings.TrimSpace(cfg.workloadAPITrustDomain) == "" {
		return nil, ErrWorkloadAPITargetPathRequiresTrustDomain
	}

	td, err := spiffeid.TrustDomainFromString(strings.TrimSpace(cfg.workloadAPITrustDomain))
	if err != nil {
		return nil, err
	}

	return tlsconfig.AuthorizeMemberOf(td), nil
}

func buildWorkloadAPITargetID(trustDomain string, targetPath string) string {
	trustDomain = strings.TrimSpace(trustDomain)
	targetPath = strings.TrimSpace(targetPath)
	if trustDomain == "" || targetPath == "" {
		return ""
	}

	td, err := spiffeid.TrustDomainFromString(trustDomain)
	if err != nil {
		return ""
	}

	id, err := spiffeid.FromPath(td, targetPath)
	if err != nil {
		return ""
	}

	return id.String()
}

type errorTransport struct {
	err error
}

func (e errorTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return nil, e.err
}

func (e errorTransport) CloseIdleConnections() {}

type managedTransport struct {
	transport *http.Transport
	closer    io.Closer
	closeOnce sync.Once
}

type closeIdleTransport struct {
	inner       http.RoundTripper
	closeIdleFn func()
}

func (c closeIdleTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return c.inner.RoundTrip(req)
}

func (c closeIdleTransport) CloseIdleConnections() {
	if closer, ok := c.inner.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
	if c.closeIdleFn != nil {
		c.closeIdleFn()
	}
}

func newManagedTransport(transport *http.Transport, closer *workloadapi.X509Source) *managedTransport {
	mt := &managedTransport{
		transport: transport,
		closer:    closer,
	}
	runtime.SetFinalizer(mt, func(t *managedTransport) {
		t.close()
	})
	return mt
}

func (m *managedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.transport.RoundTrip(req)
}

func (m *managedTransport) CloseIdleConnections() {
	m.transport.CloseIdleConnections()
	m.close()
}

func (m *managedTransport) close() {
	m.closeOnce.Do(func() {
		if m.closer != nil {
			_ = m.closer.Close()
		}
	})
}
