package frame

import (
	"net/http"
	"sync"
)

// RouteRegistry wraps http.ServeMux and records registered routes for introspection.
type RouteRegistry struct {
	mux    *http.ServeMux
	mu     sync.Mutex
	routes []RouteInfo
}

func NewRouteRegistry() *RouteRegistry {
	return &RouteRegistry{mux: http.NewServeMux()}
}

func (r *RouteRegistry) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	r.mu.Lock()
	r.routes = append(r.routes, RouteInfo{Path: pattern})
	r.mu.Unlock()
	r.mux.HandleFunc(pattern, handler)
}

func (r *RouteRegistry) Handle(pattern string, handler http.Handler) {
	r.mu.Lock()
	r.routes = append(r.routes, RouteInfo{Path: pattern})
	r.mu.Unlock()
	r.mux.Handle(pattern, handler)
}

func (r *RouteRegistry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
}

func (r *RouteRegistry) Routes() []RouteInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]RouteInfo, len(r.routes))
	copy(out, r.routes)
	return out
}
