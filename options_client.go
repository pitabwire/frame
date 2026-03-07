package frame

import (
	"context"

	"github.com/pitabwire/frame/client"
	"github.com/pitabwire/frame/config"
)

// HTTPClientManager obtains an instrumented http client for making appropriate calls downstream.
func (s *Service) HTTPClientManager() client.Manager {
	return s.clientManager
}

// WithHTTPClient configures the HTTP client used by the service.
// This allows customizing the HTTP client's behavior such as timeout, transport, etc.
func WithHTTPClient(opts ...client.HTTPOption) Option {
	return func(ctx context.Context, s *Service) {
		s.registerPlugin("http_client")

		effectiveOpts := append([]client.HTTPOption{}, opts...)

		traceCfg, ok := s.Config().(config.ConfigurationTraceRequests)
		if ok && traceCfg.TraceReq() {
			effectiveOpts = append(effectiveOpts, client.WithHTTPTraceRequests(), client.WithHTTPTraceRequestHeaders())
		}

		s.clientManager = client.NewManager(ctx, effectiveOpts...)
		s.AddCleanupMethod(func(_ context.Context) {
			s.clientManager.Close()
		})
	}
}
