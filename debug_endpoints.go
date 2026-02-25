package frame

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"

	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/queue"
)

type RouteInfo struct {
	Method  string `json:"method"`
	Path    string `json:"path"`
	Handler string `json:"handler"`
}

type RouteLister interface {
	Routes() []RouteInfo
}

// WithDebugEndpoints enables AI-friendly introspection endpoints.
func WithDebugEndpoints() Option {
	return func(_ context.Context, s *Service) {
		s.debugEnabled = true
		if s.debugBasePath == "" {
			s.debugBasePath = "/debug/frame"
		}
	}
}

// WithDebugEndpointsAt enables introspection endpoints at a custom base path.
func WithDebugEndpointsAt(basePath string) Option {
	return func(_ context.Context, s *Service) {
		s.debugEnabled = true
		s.debugBasePath = basePath
	}
}

func (s *Service) registerDebugEndpoints(mux *http.ServeMux) {
	if !s.debugEnabled {
		return
	}
	base := s.debugBasePath
	if base == "" {
		base = "/debug/frame"
	}

	mux.HandleFunc(base+"/config", s.debugConfig)
	mux.HandleFunc(base+"/plugins", s.debugPlugins)
	mux.HandleFunc(base+"/routes", s.debugRoutes)
	mux.HandleFunc(base+"/queues", s.debugQueues)
	mux.HandleFunc(base+"/health", s.debugHealth)
}

func (s *Service) debugConfig(w http.ResponseWriter, _ *http.Request) {
	cfgType := ""
	if s.configuration != nil {
		cfgType = reflect.TypeOf(s.configuration).String()
	}

	resp := map[string]any{
		"service_name": s.Name(),
		"environment":  s.Environment(),
		"version":      s.Version(),
		"config_type":  cfgType,
	}

	if rc, ok := s.Config().(config.ConfigurationRuntime); ok {
		resp["runtime_mode"] = rc.RuntimeMode()
		resp["service_id"] = rc.ServiceID()
		resp["service_group"] = rc.ServiceGroup()
	}

	writeJSON(w, resp)
}

func (s *Service) debugPlugins(w http.ResponseWriter, _ *http.Request) {
	resp := map[string]any{
		"plugins": s.registeredPlugins,
	}
	writeJSON(w, resp)
}

func (s *Service) debugRoutes(w http.ResponseWriter, _ *http.Request) {
	var routes []RouteInfo
	if s.routeLister != nil {
		routes = s.routeLister.Routes()
	}
	resp := map[string]any{
		"routes": routes,
	}
	writeJSON(w, resp)
}

func (s *Service) debugQueues(w http.ResponseWriter, _ *http.Request) {
	var pubs []queue.PublisherInfo
	var subs []queue.SubscriberInfo
	if qi, ok := s.queueManager.(queue.Inspector); ok {
		pubs = qi.ListPublishers()
		subs = qi.ListSubscribers()
	}
	resp := map[string]any{
		"publishers":  pubs,
		"subscribers": subs,
	}
	writeJSON(w, resp)
}

func (s *Service) debugHealth(w http.ResponseWriter, _ *http.Request) {
	resp := map[string]any{
		"checks": len(s.healthCheckers),
	}
	writeJSON(w, resp)
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	_ = enc.Encode(payload)
}
