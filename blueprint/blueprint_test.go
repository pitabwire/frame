package blueprint

import "testing"

func TestBlueprintValidateRequiresSchema(t *testing.T) {
	bp := &Blueprint{}
	if err := bp.Validate(); err == nil {
		t.Fatal("expected error for missing schema_version")
	}
}

func TestBlueprintValidateService(t *testing.T) {
	bp := &Blueprint{
		SchemaVersion: "0.1",
		Service: &ServiceSpec{
			Name: "users",
			HTTP: []HTTPRoute{{Route: "/users", Method: "GET", Handler: "GetUsers"}},
		},
	}
	if err := bp.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
