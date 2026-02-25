package openapi

import (
	"testing"
	"testing/fstest"
)

func TestRegisterFromFS(t *testing.T) {
	fsys := fstest.MapFS{
		"specs/users.json": {Data: []byte("{}")},
		"specs/ignore.txt": {Data: []byte("noop")},
	}

	reg := NewRegistry()
	if err := RegisterFromFS(reg, fsys, "specs"); err != nil {
		t.Fatalf("register from fs: %v", err)
	}

	spec, ok := reg.Get("users")
	if !ok {
		t.Fatalf("expected users spec to be registered")
	}
	if spec.Filename != "users.json" {
		t.Fatalf("unexpected filename: %s", spec.Filename)
	}
}
