package openapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type indexResponse struct {
	Specs []map[string]string `json:"specs"`
}

func TestServeIndex(t *testing.T) {
	reg := NewRegistry()
	reg.Add(Spec{Name: "orders", Filename: "orders.json", Content: []byte("{}")})
	reg.Add(Spec{Name: "users", Filename: "users.json", Content: []byte("{}")})

	req := httptest.NewRequest(http.MethodGet, "/debug/frame/openapi", nil)
	rec := httptest.NewRecorder()

	ServeIndex(reg)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var payload indexResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if len(payload.Specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(payload.Specs))
	}

	if payload.Specs[0]["name"] != "orders" || payload.Specs[1]["name"] != "users" {
		t.Fatalf("unexpected spec ordering: %+v", payload.Specs)
	}
}

func TestServeSpec(t *testing.T) {
	reg := NewRegistry()
	reg.Add(Spec{Name: "users", Filename: "users.json", Content: []byte("{\"ok\":true}")})

	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	rec := httptest.NewRecorder()

	ServeSpec(reg)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	if rec.Body.String() != "{\"ok\":true}" {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}
