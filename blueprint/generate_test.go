package blueprint_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pitabwire/frame/blueprint"
)

func TestGeneratePolylith(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n"), 0o644); err != nil {
		t.Fatalf("go.mod: %v", err)
	}

	bp := &blueprint.Blueprint{
		SchemaVersion: "0.1",
		RuntimeMode:   "polylith",
		Service: &blueprint.ServiceSpec{
			Name: "users",
			Port: ":8080",
			HTTP: []blueprint.HTTPRoute{
				{Route: "/users", Method: "GET", Handler: "GetUsers"},
			},
			Queues: []blueprint.QueueSpec{{Publisher: "events", URL: "mem://events"}},
		},
	}

	if err := bp.Generate(blueprint.GenerateOptions{OutDir: dir}); err != nil {
		t.Fatalf("generate: %v", err)
	}

	mainPath := filepath.Join(dir, "apps", "users", "cmd", "main.go")
	data, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read main: %v", err)
	}
	if !strings.Contains(string(data), "frame.NewService") {
		t.Fatalf("main.go missing NewService")
	}
}

func TestGenerateMonolith(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n"), 0o644); err != nil {
		t.Fatalf("go.mod: %v", err)
	}

	bp := &blueprint.Blueprint{
		SchemaVersion: "0.1",
		RuntimeMode:   "monolith",
		Services: []blueprint.ServiceSpec{
			{
				Name: "devices",
				Port: ":8081",
				HTTP: []blueprint.HTTPRoute{{Route: "/devices", Method: "GET", Handler: "GetDevices"}},
			},
			{
				Name: "geo",
				Port: ":8082",
				HTTP: []blueprint.HTTPRoute{{Route: "/geo", Method: "GET", Handler: "GetGeo"}},
			},
		},
	}

	if err := bp.Generate(blueprint.GenerateOptions{OutDir: dir}); err != nil {
		t.Fatalf("generate: %v", err)
	}

	mainPath := filepath.Join(dir, "cmd", "main.go")
	if _, err := os.Stat(mainPath); err != nil {
		t.Fatalf("main.go not found: %v", err)
	}
}
