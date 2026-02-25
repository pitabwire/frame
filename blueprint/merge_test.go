package blueprint_test

import (
	"testing"

	"github.com/pitabwire/frame/blueprint"
)

func TestMerge_Additive(t *testing.T) {
	base := blueprint.Blueprint{
		SchemaVersion: "0.1",
		Service: &blueprint.ServiceSpec{
			Name: "users",
			HTTP: []blueprint.HTTPRoute{
				{Name: "list", Method: "GET", Route: "/users", Handler: "List"},
			},
			Plugins: []string{"logging"},
		},
	}
	overlay := blueprint.Blueprint{
		SchemaVersion: "0.1",
		Service: &blueprint.ServiceSpec{
			Name: "users",
			HTTP: []blueprint.HTTPRoute{
				{Name: "create", Method: "POST", Route: "/users", Handler: "Create"},
			},
			Plugins: []string{"metrics"},
		},
	}

	out, err := blueprint.Merge(base, overlay)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	if out.Service == nil {
		t.Fatalf("expected service")
	}
	if len(out.Service.HTTP) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(out.Service.HTTP))
	}
	if len(out.Service.Plugins) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(out.Service.Plugins))
	}
}

func TestMerge_ReplaceRoute(t *testing.T) {
	base := blueprint.Blueprint{
		SchemaVersion: "0.1",
		Service: &blueprint.ServiceSpec{
			Name: "users",
			HTTP: []blueprint.HTTPRoute{
				{Name: "list", Method: "GET", Route: "/users", Handler: "List"},
			},
		},
	}
	overlay := blueprint.Blueprint{
		SchemaVersion: "0.1",
		Service: &blueprint.ServiceSpec{
			Name: "users",
			HTTP: []blueprint.HTTPRoute{
				{Name: "list", Method: "GET", Route: "/users", Handler: "ListV2"},
			},
		},
	}

	out, err := blueprint.Merge(base, overlay)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	if out.Service == nil {
		t.Fatalf("expected service")
	}
	if out.Service.HTTP[0].Handler != "List" {
		t.Fatalf("expected base to win on duplicate route")
	}
}

func TestMerge_ServiceMismatch(t *testing.T) {
	base := blueprint.Blueprint{SchemaVersion: "0.1", Service: &blueprint.ServiceSpec{Name: "users"}}
	overlay := blueprint.Blueprint{SchemaVersion: "0.1", Service: &blueprint.ServiceSpec{Name: "billing"}}

	_, err := blueprint.Merge(base, overlay)
	if err == nil {
		t.Fatalf("expected mismatch error")
	}
}
