package testoryketo

import (
	"context"
	"errors"
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

`
)

type dependancy struct {
	*definition.DefaultImpl
	configuration string
}

func New() definition.TestResource {
	return NewWithOpts(KetoConfiguration)
}

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
		Files: []testcontainers.ContainerFile{
			{
				Reader:            strings.NewReader(d.configuration),
				ContainerFilePath: "/home/ory/keto.yml",
				FileMode:          definition.ContainerFileMode,
			},
		},
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
		Files: []testcontainers.ContainerFile{
			{
				Reader:            strings.NewReader(d.configuration),
				ContainerFilePath: "/home/ory/keto.yml",
				FileMode:          definition.ContainerFileMode,
			},
		},
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
