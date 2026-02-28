package testoryketo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/mount"
	container "github.com/docker/docker/api/types/container"
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

// NamespaceFile represents an OPL namespace file to mount into the Keto container.
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
	tmpDir         string // temp directory for host-mounted files
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

// writeHostFiles writes the configuration and namespace files to a temp directory
// on the host, returning bind mount specs for docker. This is more reliable than
// testcontainers ContainerFile for Keto's namespace file watcher.
func (d *dependancy) writeHostFiles() ([]mount.Mount, error) {
	if d.tmpDir == "" {
		tmpDir, err := os.MkdirTemp("", "keto-test-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp dir: %w", err)
		}
		// Make traversable by all users (container runs as non-root ory user)
		if err := os.Chmod(tmpDir, 0o755); err != nil { //nolint:gosec // test dir
			return nil, fmt.Errorf("failed to chmod temp dir: %w", err)
		}
		d.tmpDir = tmpDir
	}

	// Write keto config
	configPath := filepath.Join(d.tmpDir, "keto.yml")
	if err := os.WriteFile(configPath, []byte(d.configuration), 0o644); err != nil { //nolint:gosec // test file
		return nil, fmt.Errorf("failed to write keto config: %w", err)
	}

	mounts := []mount.Mount{
		{
			Type:   mount.TypeBind,
			Source: configPath,
			Target: "/home/ory/keto.yml",
		},
	}

	// Write namespace files
	for i, ns := range d.namespaceFiles {
		nsDir := filepath.Join(d.tmpDir, fmt.Sprintf("ns_%d", i))
		if err := os.MkdirAll(nsDir, 0o755); err != nil { //nolint:gosec,gocritic // test dir
			return nil, fmt.Errorf("failed to create namespace dir: %w", err)
		}

		nsPath := filepath.Join(nsDir, filepath.Base(ns.ContainerPath))
		if err := os.WriteFile(nsPath, []byte(ns.Content), 0o644); err != nil { //nolint:gosec // test file
			return nil, fmt.Errorf("failed to write namespace file: %w", err)
		}

		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: nsPath,
			Target: ns.ContainerPath,
		})
	}

	return mounts, nil
}

// containerFiles returns file copies including the config and any namespace files.
// Used for containers that need config parsing but not OPL file watching (e.g. migration).
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

func (d *dependancy) Setup(ctx context.Context, ntwk *testcontainers.DockerNetwork) error {
	if len(d.Opts().Dependencies) == 0 || !d.Opts().Dependencies[0].GetDS(ctx).IsDB() {
		return errors.New("no ByIsDatabase dependencies was supplied")
	}

	ketoDB, _, err := testpostgres.CreateDatabase(ctx, d.Opts().Dependencies[0].GetInternalDS(ctx), "keto")
	if err != nil {
		return err
	}

	databaseURL := ketoDB.String()

	err = d.migrateContainer(ctx, ntwk, databaseURL)
	if err != nil {
		return err
	}

	containerRequest := testcontainers.ContainerRequest{
		Image: d.Name(),
		Cmd:   []string{"serve", "--config", "/home/ory/keto.yml"},
		Env: d.Opts().Env(map[string]string{
			"LOG_LEVEL":                 "debug",
			"LOG_LEAK_SENSITIVE_VALUES": "true",
			"DSN":                       databaseURL,
		}),
		WaitingFor: wait.ForHTTP("/health/ready").WithPort(d.DefaultPort),
	}

	// Use bind mounts for the serve container when namespace files are present,
	// as Keto's namespace file watcher requires host-mounted files to work
	// correctly with OPL evaluation.
	if len(d.namespaceFiles) > 0 {
		mounts, mountErr := d.writeHostFiles()
		if mountErr != nil {
			return fmt.Errorf("failed to write host files: %w", mountErr)
		}
		containerRequest.HostConfigModifier = func(hc *container.HostConfig) {
			hc.Mounts = append(hc.Mounts, mounts...)
		}
	} else {
		containerRequest.Files = d.containerFiles()
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

func (d *dependancy) Cleanup(ctx context.Context) {
	d.DefaultImpl.Cleanup(ctx)

	// Clean up temp files
	if d.tmpDir != "" {
		_ = os.RemoveAll(d.tmpDir)
	}
}
