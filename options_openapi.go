package frame

import (
	"context"
	"fmt"
	"io/fs"
	"strings"

	"github.com/pitabwire/frame/openapi"
)

// WithOpenAPISpec registers a single OpenAPI spec with the service.
// Content should be provided from compile-time embedded assets.
func WithOpenAPISpec(name, filename string, content []byte) Option {
	return func(_ context.Context, s *Service) {
		if s.openapiRegistry == nil {
			s.openapiRegistry = openapi.NewRegistry()
		}
		s.openapiRegistry.Add(openapi.Spec{Name: name, Filename: filename, Content: content})
	}
}

// WithOpenAPISpecsFromFS registers all .json OpenAPI specs from an embedded FS directory.
// Use //go:embed to provide the fs.FS at compile time.
func WithOpenAPISpecsFromFS(f fs.FS, dir string) Option {
	return func(ctx context.Context, s *Service) {
		if s.openapiRegistry == nil {
			s.openapiRegistry = openapi.NewRegistry()
		}
		if err := openapi.RegisterFromFS(s.openapiRegistry, f, dir); err != nil {
			s.AddStartupError(fmt.Errorf("openapi register from fs: %w", err))
			s.Log(ctx).Error(err.Error())
		}
	}
}

// WithOpenAPIBasePath sets the base path used to serve OpenAPI specs.
// The default is /debug/frame/openapi.
func WithOpenAPIBasePath(path string) Option {
	return func(_ context.Context, s *Service) {
		s.openapiBasePath = normalizeOpenAPIBasePath(path)
	}
}

func (s *Service) OpenAPIRegistry() *openapi.Registry {
	return s.openapiRegistry
}

func normalizeOpenAPIBasePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return defaultOpenAPIBasePath
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return strings.TrimSuffix(path, "/")
}
