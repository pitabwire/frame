package frame

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// OPLSpec holds an OPL namespace definition registered by a service.
type OPLSpec struct {
	Namespace string
	Data      []byte
}

// WithOPL registers an OPL (Open Policy Language) namespace definition to be
// served at /_internal/opl/{namespace}. Multiple OPL specs can be registered
// for services that span multiple namespaces.
func WithOPL(namespace string, data []byte) Option {
	return func(_ context.Context, s *Service) {
		if s.oplSpecs == nil {
			s.oplSpecs = make(map[string][]byte)
		}
		s.oplSpecs[namespace] = data
	}
}

func (s *Service) registerOPLEndpoints(mux *http.ServeMux) {
	if len(s.oplSpecs) == 0 {
		return
	}

	mux.HandleFunc("/_internal/opl/", s.handleOPL)
	mux.HandleFunc("/_internal/opl", s.handleOPLList)
}

func (s *Service) handleOPL(w http.ResponseWriter, r *http.Request) {
	namespace := strings.TrimPrefix(r.URL.Path, "/_internal/opl/")
	namespace = strings.TrimSuffix(namespace, "/")

	if namespace == "" {
		s.handleOPLList(w, r)
		return
	}

	data, ok := s.oplSpecs[namespace]
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(data)
}

func (s *Service) handleOPLList(w http.ResponseWriter, _ *http.Request) {
	namespaces := make([]string, 0, len(s.oplSpecs))
	for ns := range s.oplSpecs {
		namespaces = append(namespaces, ns)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"namespaces": namespaces,
	})
}
