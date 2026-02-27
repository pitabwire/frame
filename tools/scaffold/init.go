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
	module := resolveModule(root, cfg.Module, len(services) > 0)

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

	if module != "" {
		if modErr := writeGoMod(root, module); modErr != nil {
			return modErr
		}
	}

	for _, svc := range services {
		if svcErr := writeServiceLayout(root, svc, module); svcErr != nil {
			return svcErr
		}
	}

	if entryErr := writeRepoEntrypoints(root, services, module); entryErr != nil {
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
	content := `ARG TARGETOS=linux
ARG TARGETARCH=amd64

# ---------- Builder ----------
FROM golang:1.26 AS builder
WORKDIR /app

ARG APP=users
ARG REPOSITORY
ARG VERSION=dev
ARG REVISION=none
ARG BUILDTIME

COPY go.mod go.sum ./
RUN go mod download

COPY ./cmd ./cmd
COPY ./apps ./apps
COPY ./pkg ./pkg

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
	go build -trimpath \
	-ldflags="-s -w \
	-X github.com/pitabwire/frame/version.Repository=${REPOSITORY} \
	-X github.com/pitabwire/frame/version.Version=${VERSION} \
	-X github.com/pitabwire/frame/version.Commit=${REVISION} \
	-X github.com/pitabwire/frame/version.Date=${BUILDTIME}" \
	-o /app/binary ./cmd/${APP}/main.go

# ---------- Final ----------
FROM cgr.dev/chainguard/static:latest
USER 65532:65532
WORKDIR /
COPY --from=builder /app/binary /service
ENTRYPOINT ["/service"]
`
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

func writeServiceLayout(root, name, module string) error {
	base := filepath.Join(root, "apps", name)
	paths := []string{
		filepath.Join(base, "cmd"),
		filepath.Join(base, "service"),
		filepath.Join(base, "queues"),
		filepath.Join(base, "config"),
		filepath.Join(base, "migrations"),
		filepath.Join(base, "tests"),
	}
	for _, p := range paths {
		if dirErr := ensureDir(p); dirErr != nil {
			return dirErr
		}
	}

	mainPath := filepath.Join(base, "cmd", "main.go")
	if !exists(mainPath) {
		content := fmt.Sprintf(`package main

import (
	"log"
	"net/http"
	"github.com/pitabwire/frame"
	"%s/apps/%s/service"
)

func main() {
	mux := http.NewServeMux()
	service.RegisterRoutes(mux)
	ctx, svc := frame.NewService(
		frame.WithName("%s"),
		frame.WithHTTPHandler(mux),
	)
	if err := svc.Run(ctx, ":8080"); err != nil {
		log.Fatal(err)
	}
}
`, module, name, name)
		// #nosec G306 -- generated files should be readable by the developer.
		if writeErr := os.WriteFile(mainPath, []byte(content), 0o644); writeErr != nil {
			return fmt.Errorf("write %s: %w", mainPath, writeErr)
		}
	}

	dockerPath := filepath.Join(base, "Dockerfile")
	if !exists(dockerPath) {
		content := fmt.Sprintf(`ARG TARGETOS=linux
ARG TARGETARCH=amd64

# ---------- Builder ----------
FROM golang:1.26 AS builder
WORKDIR /app

ARG REPOSITORY
ARG VERSION=dev
ARG REVISION=none
ARG BUILDTIME

COPY go.mod go.sum ./
RUN go mod download

COPY ./apps/%s ./apps/%s
COPY ./pkg ./pkg

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
	go build -trimpath \
	-ldflags="-s -w \
	-X github.com/pitabwire/frame/version.Repository=${REPOSITORY} \
	-X github.com/pitabwire/frame/version.Version=${VERSION} \
	-X github.com/pitabwire/frame/version.Commit=${REVISION} \
	-X github.com/pitabwire/frame/version.Date=${BUILDTIME}" \
	-o /app/binary ./apps/%s/cmd/main.go

# ---------- Final ----------
FROM cgr.dev/chainguard/static:latest
USER 65532:65532
WORKDIR /
COPY --from=builder /app/binary /service
ENTRYPOINT ["/service"]
`, name, name, name)
		// #nosec G306 -- generated files should be readable by the developer.
		if writeErr := os.WriteFile(dockerPath, []byte(content), 0o644); writeErr != nil {
			return fmt.Errorf("write %s: %w", dockerPath, writeErr)
		}
	}

	if routeErr := writeServiceRoutes(root, name); routeErr != nil {
		return routeErr
	}

	return nil
}

func writeServiceRoutes(root, name string) error {
	serviceDir := filepath.Join(root, "apps", name, "service")
	if err := ensureDir(serviceDir); err != nil {
		return err
	}

	routesPath := filepath.Join(serviceDir, "routes.go")
	if exists(routesPath) {
		return nil
	}

	content := `package service

import "net/http"

// RegisterRoutes wires service HTTP endpoints.
func RegisterRoutes(mux *http.ServeMux) {
	_ = mux
}
`
	// #nosec G306 -- generated files should be readable by the developer.
	return os.WriteFile(routesPath, []byte(content), 0o644)
}

func writeRepoEntrypoints(root string, services []string, module string) error {
	for _, svc := range services {
		path := filepath.Join(root, "cmd", svc, "main.go")
		if exists(path) {
			continue
		}
		if err := ensureDir(filepath.Join(root, "cmd", svc)); err != nil {
			return err
		}

		content := fmt.Sprintf(`package main

import (
	"log"
	"net/http"
	"github.com/pitabwire/frame"
	"%s/apps/%s/service"
)

func main() {
	mux := http.NewServeMux()
	service.RegisterRoutes(mux)

	ctx, svc := frame.NewService(
		frame.WithName("%s"),
		frame.WithHTTPHandler(mux),
	)
	if err := svc.Run(ctx, ":8080"); err != nil {
		log.Fatal(err)
	}
}
`, module, svc, svc)

		// #nosec G306 -- generated files should be readable by the developer.
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

func resolveModule(root, moduleFlag string, needModule bool) string {
	module := strings.TrimSpace(moduleFlag)
	if module != "" {
		return module
	}

	if existing, err := moduleFromGoMod(root); err == nil && existing != "" {
		return existing
	}

	if !needModule {
		return ""
	}

	// fallback to keep scaffolding usable even before module is initialized.
	return "example.com/project"
}

func moduleFromGoMod(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	return "", errors.New("module path not found in go.mod")
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
