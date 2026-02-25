package openapi

import (
	"encoding/json"
	"net/http"
	"strings"
)

func ServeIndex(reg *Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		items := reg.List()
		index := make([]map[string]string, 0, len(items))
		for _, s := range items {
			index = append(index, map[string]string{
				"name":     s.Name,
				"filename": s.Filename,
			})
		}
		writeJSON(w, map[string]any{"specs": index})
	}
}

func ServeSpec(reg *Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		spec, ok := reg.Get(name)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// #nosec G705 -- serving trusted embedded OpenAPI specs.
		if _, err := w.Write(spec.Content); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	_ = enc.Encode(payload)
}
