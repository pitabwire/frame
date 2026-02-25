package openapi

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// RegisterFromFS registers all .json specs from an embedded FS directory.
func RegisterFromFS(reg *Registry, f fs.FS, dir string) error {
	entries, err := fs.ReadDir(f, dir)
	if err != nil {
		return err
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".json") {
			continue
		}
		data, readErr := fs.ReadFile(f, filepath.Join(dir, name))
		if readErr != nil {
			return readErr
		}
		specName := strings.TrimSuffix(name, filepath.Ext(name))
		reg.Add(Spec{Name: specName, Filename: name, Content: data})
	}

	return nil
}
