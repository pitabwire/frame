package blueprint

import "testing"

func TestMerge_Additive(t *testing.T) {
	base := Blueprint{
		Service: "users",
		HTTP:    []HTTPRoute{{Name: "list", Method: "GET", Route: "/users", Handler: "List"}},
		Plugins: []Plugin{{Name: "logging"}},
	}
	overlay := Blueprint{
		HTTP:    []HTTPRoute{{Name: "create", Method: "POST", Route: "/users", Handler: "Create"}},
		Plugins: []Plugin{{Name: "metrics"}},
	}

	out, err := Merge(base, overlay)
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
	base := Blueprint{HTTP: []HTTPRoute{{Name: "list", Method: "GET", Route: "/users", Handler: "List"}}}
	overlay := Blueprint{HTTP: []HTTPRoute{{Name: "list", Method: "GET", Route: "/users", Handler: "ListV2", Override: true}}}

	out, err := Merge(base, overlay)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	if out.HTTP[0].Handler != "ListV2" {
		t.Fatalf("expected override to apply")
	}
}

func TestMerge_Remove(t *testing.T) {
	base := Blueprint{HTTP: []HTTPRoute{{Name: "list", Method: "GET", Route: "/users", Handler: "List"}}}
	overlay := Blueprint{HTTP: []HTTPRoute{{Name: "list", Remove: true}}}

	out, err := Merge(base, overlay)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	if len(out.HTTP) != 0 {
		t.Fatalf("expected route removed, got %d", len(out.HTTP))
	}
}

func TestMerge_ServiceMismatch(t *testing.T) {
	base := Blueprint{Service: "users"}
	overlay := Blueprint{Service: "billing"}

	_, err := Merge(base, overlay)
	if err == nil {
		t.Fatalf("expected mismatch error")
	}
}
