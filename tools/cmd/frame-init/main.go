package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type config struct {
	Root     string
	Services string
	Module   string
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.Root, "root", ".", "Repository root")
	flag.StringVar(&cfg.Services, "services", "", "Comma-separated service names")
	flag.StringVar(&cfg.Module, "module", "", "Go module path (optional)")
	flag.Parse()

	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func run(cfg config) error {
	root, err := filepath.Abs(cfg.Root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}

	services := parseServices(cfg.Services)

	repoDirs := []string{
		"apps",
		"cmd",
		"pkg",
		"configs",
	}
	for _, dir := range repoDirs {
		if err := ensureDir(filepath.Join(root, dir)); err != nil {
			return err
		}
	}

	if err := writeRepoDockerfile(root); err != nil {
		return err
	}

	if cfg.Module != "" {
		if err := writeGoMod(root, cfg.Module); err != nil {
			return err
		}
	}

	for _, svc := range services {
		if err := writeServiceLayout(root, svc); err != nil {
			return err
		}
	}

	if err := writeMonolithEntry(root, services); err != nil {
		return err
	}

	return nil
}

func parseServices(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		name := strings.TrimSpace(p)
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	return out
}

func ensureDir(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create dir %s: %w", path, err)
	}
	return nil
}

func writeRepoDockerfile(root string) error {
	path := filepath.Join(root, "Dockerfile")
	if exists(path) {
		return nil
	}
	content := "FROM golang:1.22-alpine\nWORKDIR /app\nCOPY . .\nRUN go build -o /app/monolith ./cmd/monolith\nCMD [\"/app/monolith\"]\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

func writeGoMod(root, module string) error {
	path := filepath.Join(root, "go.mod")
	if exists(path) {
		return nil
	}
	content := fmt.Sprintf("module %s\n\ngo 1.22\n", module)
	return os.WriteFile(path, []byte(content), 0o644)
}

func writeServiceLayout(root, name string) error {
	base := filepath.Join(root, "apps", name)
	paths := []string{
		filepath.Join(base, "cmd", name),
		filepath.Join(base, "service"),
		filepath.Join(base, "config"),
		filepath.Join(base, "migrations"),
		filepath.Join(base, "tests"),
	}
	for _, p := range paths {
		if err := ensureDir(p); err != nil {
			return err
		}
	}

	mainPath := filepath.Join(base, "cmd", name, "main.go")
	if !exists(mainPath) {
		content := fmt.Sprintf("package main\n\nimport (\n\t\"log\"\n\n\t\"github.com/pitabwire/frame\"\n)\n\nfunc main() {\n\tctx, svc := frame.NewService(\n\t\tframe.WithName(\"%s\"),\n\t)\n\tif err := svc.Run(ctx, \":8080\"); err != nil {\n\t\tlog.Fatal(err)\n\t}\n}\n", name)
		if err := os.WriteFile(mainPath, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", mainPath, err)
		}
	}

	dockerPath := filepath.Join(base, "cmd", name, "Dockerfile")
	if !exists(dockerPath) {
		content := fmt.Sprintf("FROM golang:1.22-alpine\nWORKDIR /app\nCOPY . .\nRUN go build -o /app/%s ./apps/%s/cmd/%s\nCMD [\"/app/%s\"]\n", name, name, name, name)
		if err := os.WriteFile(dockerPath, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", dockerPath, err)
		}
	}

	return nil
}

func writeMonolithEntry(root string, services []string) error {
	path := filepath.Join(root, "cmd", "monolith", "main.go")
	if exists(path) {
		return nil
	}
	if err := ensureDir(filepath.Join(root, "cmd", "monolith")); err != nil {
		return err
	}

	imports := []string{"\"log\"", "\"net/http\"", "\"github.com/pitabwire/frame\""}
	for _, svc := range services {
		imports = append(imports, fmt.Sprintf("\"%s/apps/%s/service\"", "your/module", svc))
	}

	var builders strings.Builder
	builders.WriteString("package main\n\nimport (\n")
	for _, imp := range imports {
		builders.WriteString("\t" + imp + "\n")
	}
	builders.WriteString(")\n\nfunc main() {\n\tmux := http.NewServeMux()\n")
	for _, svc := range services {
		builders.WriteString(fmt.Sprintf("\t%s.RegisterRoutes(mux)\n", svc))
	}
	builders.WriteString("\tctx, svc := frame.NewService(\n\t\tframe.WithName(\"monolith\"),\n\t\tframe.WithHTTPHandler(mux),\n\t)\n\tif err := svc.Run(ctx, \":8080\"); err != nil {\n\t\tlog.Fatal(err)\n\t}\n}\n")

	return os.WriteFile(path, []byte(builders.String()), 0o644)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
