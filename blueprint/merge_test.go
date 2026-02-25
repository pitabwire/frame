package blueprint_test

import (
	"testing"

	"github.com/pitabwire/frame/blueprint"
)

func TestMerge_Additive(t *testing.T) {
	base := blueprint.Blueprint{
		Service: "users",
		HTTP: []blueprint.HTTPRoute{
			{Name: "list", Method: "GET", Route: "/users", Handler: "List"},
		},
		Plugins: []blueprint.Plugin{{Name: "logging"}},
	}
	overlay := blueprint.Blueprint{
		HTTP: []blueprint.HTTPRoute{
			{Name: "create", Method: "POST", Route: "/users", Handler: "Create"},
		},
		Plugins: []blueprint.Plugin{{Name: "metrics"}},
	}

	out, err := blueprint.Merge(base, overlay)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	if len(out.HTTP) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(out.HTTP))
	}
	if len(out.Plugins) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(out.Plugins))
	}
}

func TestMerge_Override(t *testing.T) {
	base := blueprint.Blueprint{
		HTTP: []blueprint.HTTPRoute{
			{Name: "list", Method: "GET", Route: "/users", Handler: "List"},
		},
	}
	overlay := blueprint.Blueprint{
		HTTP: []blueprint.HTTPRoute{
			{Name: "list", Method: "GET", Route: "/users", Handler: "ListV2", Override: true},
		},
	}

	out, err := blueprint.Merge(base, overlay)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	if out.HTTP[0].Handler != "ListV2" {
		t.Fatalf("expected override to apply")
	}
}

func TestMerge_Remove(t *testing.T) {
	base := blueprint.Blueprint{
		HTTP: []blueprint.HTTPRoute{
			{Name: "list", Method: "GET", Route: "/users", Handler: "List"},
		},
	}
	overlay := blueprint.Blueprint{
		HTTP: []blueprint.HTTPRoute{
			{Name: "list", Remove: true},
		},
	}

	out, err := blueprint.Merge(base, overlay)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	if len(out.HTTP) != 0 {
		t.Fatalf("expected route removed, got %d", len(out.HTTP))
	}
}

func TestMerge_ServiceMismatch(t *testing.T) {
	base := blueprint.Blueprint{Service: "users"}
	overlay := blueprint.Blueprint{Service: "billing"}

	_, err := blueprint.Merge(base, overlay)
	if err == nil {
		t.Fatalf("expected mismatch error")
	}
}
