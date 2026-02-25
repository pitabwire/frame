package blueprint_test

import (
	"testing"

	"github.com/pitabwire/frame/blueprint"
)

func TestBlueprintValidateRequiresSchema(t *testing.T) {
	bp := &blueprint.Blueprint{}
	if err := bp.Validate(); err == nil {
		t.Fatal("expected error for missing schema_version")
	}
}

func TestBlueprintValidateService(t *testing.T) {
	bp := &blueprint.Blueprint{
		SchemaVersion: "0.1",
		Service: &blueprint.ServiceSpec{
			Name: "users",
			HTTP: []blueprint.HTTPRoute{
				{Route: "/users", Method: "GET", Handler: "GetUsers"},
			},
		},
	}
	if err := bp.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
