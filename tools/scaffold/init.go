package scaffold

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type InitConfig struct {
	Root     string
	Services string
	Module   string
}

func DefaultInitConfig() InitConfig {
	return InitConfig{
		Root: ".",
	}
}

func InitRepo(cfg InitConfig) error {
	root, err := filepath.Abs(cfg.Root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}

	services := parseServices(cfg.Services)
	if len(services) > 0 && strings.TrimSpace(cfg.Module) == "" {
		return errors.New("module is required when services are provided")
	}

	repoDirs := []string{
		"apps",
		"cmd",
		"pkg",
		"configs",
	}
	for _, dir := range repoDirs {
		if dirErr := ensureDir(filepath.Join(root, dir)); dirErr != nil {
			return dirErr
		}
	}

	if dockerErr := writeRepoDockerfile(root); dockerErr != nil {
		return dockerErr
	}

	if cfg.Module != "" {
		if modErr := writeGoMod(root, cfg.Module); modErr != nil {
			return modErr
		}
	}

	for _, svc := range services {
		if svcErr := writeServiceLayout(root, svc); svcErr != nil {
			return svcErr
		}
	}

	if entryErr := writeMonolithEntry(root, services, cfg.Module); entryErr != nil {
		return entryErr
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
	// #nosec G301 -- scaffold output should be readable by the developer.
	if mkErr := os.MkdirAll(path, 0o755); mkErr != nil {
		return fmt.Errorf("create dir %s: %w", path, mkErr)
	}
	return nil
}

func writeRepoDockerfile(root string) error {
	path := filepath.Join(root, "Dockerfile")
	if exists(path) {
		return nil
	}
	content := "FROM golang:1.22-alpine\nWORKDIR /app\nCOPY . .\nRUN go build -o /app/monolith ./cmd/monolith\nCMD [\"/app/monolith\"]\n"
	// #nosec G306 -- generated files should be readable by the developer.
	return os.WriteFile(path, []byte(content), 0o644)
}

func writeGoMod(root, module string) error {
	path := filepath.Join(root, "go.mod")
	if exists(path) {
		return nil
	}
	content := fmt.Sprintf("module %s\n\ngo 1.22\n", module)
	// #nosec G306 -- generated files should be readable by the developer.
	return os.WriteFile(path, []byte(content), 0o644)
}

func writeServiceLayout(root, name string) error {
	base := filepath.Join(root, "apps", name)
	paths := []string{
		filepath.Join(base, "cmd", name),
		filepath.Join(base, "pkg"),
		filepath.Join(base, "config"),
		filepath.Join(base, "migrations"),
		filepath.Join(base, "tests"),
	}
	for _, p := range paths {
		if dirErr := ensureDir(p); dirErr != nil {
			return dirErr
		}
	}

	mainPath := filepath.Join(base, "cmd", name, "main.go")
	if !exists(mainPath) {
		content := fmt.Sprintf(`package main

import (
	"log"

	"github.com/pitabwire/frame"
)

func main() {
	ctx, svc := frame.NewService(
		frame.WithName("%s"),
	)
	if err := svc.Run(ctx, ":8080"); err != nil {
		log.Fatal(err)
	}
}
`, name)
		// #nosec G306 -- generated files should be readable by the developer.
		if writeErr := os.WriteFile(mainPath, []byte(content), 0o644); writeErr != nil {
			return fmt.Errorf("write %s: %w", mainPath, writeErr)
		}
	}

	dockerPath := filepath.Join(base, "cmd", name, "Dockerfile")
	if !exists(dockerPath) {
		content := fmt.Sprintf(`FROM golang:1.22-alpine
WORKDIR /app
COPY . .
RUN go build -o /app/%s ./apps/%s/cmd/%s
CMD ["/app/%s"]
`, name, name, name, name)
		// #nosec G306 -- generated files should be readable by the developer.
		if writeErr := os.WriteFile(dockerPath, []byte(content), 0o644); writeErr != nil {
			return fmt.Errorf("write %s: %w", dockerPath, writeErr)
		}
	}

	return nil
}

func writeMonolithEntry(root string, services []string, module string) error {
	path := filepath.Join(root, "cmd", "monolith", "main.go")
	if exists(path) {
		return nil
	}
	if err := ensureDir(filepath.Join(root, "cmd", "monolith")); err != nil {
		return err
	}

	imports := []string{"\"log\"", "\"net/http\"", "\"github.com/pitabwire/frame\""}
	for _, svc := range services {
		alias := sanitizeImportAlias(svc)
		imports = append(imports, fmt.Sprintf("%s \"%s/apps/%s/pkg\"", alias, module, svc))
	}

	var builders strings.Builder
	builders.WriteString("package main\n\nimport (\n")
	for _, imp := range imports {
		builders.WriteString("\t" + imp + "\n")
	}
	builders.WriteString(")\n\nfunc main() {\n\tmux := http.NewServeMux()\n")
	for _, svc := range services {
		fmt.Fprintf(&builders, "\t%s.RegisterRoutes(mux)\n", sanitizeImportAlias(svc))
	}
	builders.WriteString("\tctx, svc := frame.NewService(\n")
	builders.WriteString("\t\tframe.WithName(\"monolith\"),\n")
	builders.WriteString("\t\tframe.WithHTTPHandler(mux),\n")
	builders.WriteString("\t)\n")
	builders.WriteString("\tif err := svc.Run(ctx, \":8080\"); err != nil {\n")
	builders.WriteString("\t\tlog.Fatal(err)\n")
	builders.WriteString("\t}\n")
	builders.WriteString("}\n")

	// #nosec G306 -- generated files should be readable by the developer.
	return os.WriteFile(path, []byte(builders.String()), 0o644)
}

func sanitizeImportAlias(name string) string {
	if name == "" {
		return "svc"
	}
	var out strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			out.WriteRune(r)
			continue
		}
		out.WriteRune('_')
	}
	alias := out.String()
	if alias[0] >= '0' && alias[0] <= '9' {
		return "svc_" + alias
	}
	return alias
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
