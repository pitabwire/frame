package testoryketo

import (
	"context"
	"fmt"
	"strings"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/frametests/deps/testpostgres"
)

const (
	// OryKetoImage is the Ory Keto Image.
	OryKetoImage = "oryd/keto:latest"

	KetoConfiguration = `
version: v0.14.0

dsn: memory

serve:
  read:
    host: 0.0.0.0
    port: 4466
  write:
    host: 0.0.0.0
    port: 4467

log:
  level: debug
  format: text

namespaces:
  - id: 0
    name: default
  - id: 1
    name: partition
  - id: 2
    name: default/profile
  - id: 3
    name: profile
  - id: 4
    name: custom

`
)

// NamespaceFile represents an OPL namespace file to copy into the Keto container.
type NamespaceFile struct {
	// ContainerPath is the absolute path inside the container (e.g. "/home/ory/namespaces/tenancy.ts").
	ContainerPath string
	// Content is the OPL TypeScript content.
	Content string
}

type dependancy struct {
	*definition.DefaultImpl
	configuration  string
	namespaceFiles []NamespaceFile
}

func New() definition.TestResource {
	return NewWithOpts(KetoConfiguration)
}

// NewWithOpts creates a new Keto test resource.
// The configuration string is the Keto YAML config.
func NewWithOpts(
	configuration string,
	containerOpts ...definition.ContainerOption,
) definition.TestResource {
	opts := definition.ContainerOpts{
		ImageName:      OryKetoImage,
		Ports:          []string{"4467/tcp", "4466/tcp"},
		NetworkAliases: []string{"keto", "auth-keto"},
	}
	opts.Setup(containerOpts...)

	return &dependancy{
		DefaultImpl:   definition.NewDefaultImpl(opts, "http"),
		configuration: configuration,
	}
}

// NewWithNamespaces creates a new Keto test resource with OPL namespace files.
// The configuration string should reference the namespace files via
// "namespaces: location: file:///path/to/file.ts".
// When no database dependency is provided, Keto uses in-memory storage
// with auto-migration, which is required for OPL permit evaluation.
func NewWithNamespaces(
	configuration string,
	namespaceFiles []NamespaceFile,
	containerOpts ...definition.ContainerOption,
) definition.TestResource {
	opts := definition.ContainerOpts{
		ImageName:      OryKetoImage,
		Ports:          []string{"4467/tcp", "4466/tcp"},
		NetworkAliases: []string{"keto", "auth-keto"},
	}
	opts.Setup(containerOpts...)

	return &dependancy{
		DefaultImpl:    definition.NewDefaultImpl(opts, "http"),
		configuration:  configuration,
		namespaceFiles: namespaceFiles,
	}
}

// containerFiles returns file copies including the config and any namespace files.
func (d *dependancy) containerFiles() []testcontainers.ContainerFile {
	files := []testcontainers.ContainerFile{
		{
			Reader:            strings.NewReader(d.configuration),
			ContainerFilePath: "/home/ory/keto.yml",
			FileMode:          definition.ContainerFileMode,
		},
	}
	for _, ns := range d.namespaceFiles {
		files = append(files, testcontainers.ContainerFile{
			Reader:            strings.NewReader(ns.Content),
			ContainerFilePath: ns.ContainerPath,
			FileMode:          definition.ContainerFileMode,
		})
	}
	return files
}

func (d *dependancy) migrateContainer(
	ctx context.Context,
	ntwk *testcontainers.DockerNetwork,
	databaseURL string,
) error {
	containerRequest := testcontainers.ContainerRequest{
		Image: d.Name(),
		Cmd:   []string{"migrate", "up", "--yes"},
		Env: map[string]string{
			"LOG_LEVEL": "debug",
			"DSN":       databaseURL,
		},
		Files:      d.containerFiles(),
		WaitingFor: wait.ForExit(),
	}

	d.Configure(ctx, ntwk, &containerRequest)

	ketoContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: containerRequest,
		Started:          true,
	})
	if err != nil {
		return err
	}

	err = ketoContainer.Terminate(ctx)
	if err != nil {
		return err
	}
	return nil
}

// hasDBDependency checks if a database dependency is available.
func (d *dependancy) hasDBDependency(ctx context.Context) bool {
	deps := d.Opts().Dependencies
	return len(deps) > 0 && deps[0].GetDS(ctx).IsDB()
}

func (d *dependancy) Setup(ctx context.Context, ntwk *testcontainers.DockerNetwork) error {
	env := d.Opts().Env(map[string]string{
		"LOG_LEVEL":                 "debug",
		"LOG_LEAK_SENSITIVE_VALUES": "true",
	})

	// When a database dependency is available, use it with separate migration.
	// Otherwise, use in-memory storage with auto-migration (supports OPL evaluation).
	if d.hasDBDependency(ctx) {
		ketoDB, _, err := testpostgres.CreateDatabase(ctx, d.Opts().Dependencies[0].GetInternalDS(ctx), "keto")
		if err != nil {
			return err
		}

		databaseURL := ketoDB.String()
		env["DSN"] = databaseURL

		err = d.migrateContainer(ctx, ntwk, databaseURL)
		if err != nil {
			return err
		}
	}

	containerRequest := testcontainers.ContainerRequest{
		Image:      d.Name(),
		Cmd:        []string{"serve", "--config", "/home/ory/keto.yml"},
		Env:        env,
		Files:      d.containerFiles(),
		WaitingFor: wait.ForHTTP("/health/ready").WithPort(d.DefaultPort),
	}

	d.Configure(ctx, ntwk, &containerRequest)

	ketoContainer, err := testcontainers.GenericContainer(ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: containerRequest,
			Started:          true,
		})

	if err != nil {
		return fmt.Errorf("failed to start ketoContainer: %w", err)
	}

	d.SetContainer(ketoContainer)
	return nil
}
