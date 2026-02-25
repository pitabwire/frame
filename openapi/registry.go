package openapi

import (
	"sort"
	"sync"
)

type Spec struct {
	Name     string
	Filename string
	Content  []byte
}

type Registry struct {
	mu    sync.RWMutex
	specs map[string]Spec
}

func NewRegistry() *Registry {
	return &Registry{specs: map[string]Spec{}}
}

func (r *Registry) Add(spec Spec) {
	if spec.Name == "" || len(spec.Content) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.specs[spec.Name] = spec
}

func (r *Registry) Get(name string) (Spec, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.specs[name]
	return s, ok
}

func (r *Registry) List() []Spec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Spec, 0, len(r.specs))
	for _, s := range r.specs {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
