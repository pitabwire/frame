package frame

import (
	"net/http"
	"reflect"
	"runtime"
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
	r.routes = append(r.routes, RouteInfo{
		Path:    pattern,
		Handler: handlerName(handler),
	})
	r.mu.Unlock()
	r.mux.HandleFunc(pattern, handler)
}

func (r *RouteRegistry) Handle(pattern string, handler http.Handler) {
	r.mu.Lock()
	r.routes = append(r.routes, RouteInfo{
		Path:    pattern,
		Handler: reflect.TypeOf(handler).String(),
	})
	r.mu.Unlock()
	r.mux.Handle(pattern, handler)
}

// HandleRoute records method-aware routes for introspection.
func (r *RouteRegistry) HandleRoute(method, pattern, name string, handler func(http.ResponseWriter, *http.Request)) {
	if name == "" {
		name = handlerName(handler)
	}
	r.mu.Lock()
	r.routes = append(r.routes, RouteInfo{
		Method:  method,
		Path:    pattern,
		Handler: name,
	})
	r.mu.Unlock()
	r.mux.HandleFunc(pattern, handler)
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

func handlerName(handler func(http.ResponseWriter, *http.Request)) string {
	if handler == nil {
		return ""
	}
	ptr := reflect.ValueOf(handler).Pointer()
	if ptr == 0 {
		return ""
	}
	if fn := runtime.FuncForPC(ptr); fn != nil {
		return fn.Name()
	}
	return ""
}
