package testoryketo

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/docker/go-connections/nat"
	"github.com/pitabwire/util"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/frametests/definition"
)

const (
	// OryKetoImage is the Ory Keto Image.
	OryKetoImage = "oryd/keto:latest"

	KetoConfiguration = `
version: v0.12.0

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
    name: files
    config:
      location: file://etc/config/keto_namespaces

`
)

type ketoDependancy struct {
	opts          definition.ContainerOpts
	configuration string
	conn          frame.DataSource
	internalConn  frame.DataSource

	container testcontainers.Container
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
		Ports:          []string{"4466", "4467"},
		NetworkAliases: []string{"keto", "auth-keto"},
	}
	opts.Setup(containerOpts...)

	return &ketoDependancy{
		opts:          opts,
		configuration: configuration,
	}
}

func (d *ketoDependancy) Name() string {
	return d.opts.ImageName
}

func (d *ketoDependancy) Container() testcontainers.Container {
	return d.container
}

func (d *ketoDependancy) migrateContainer(
	ctx context.Context,
	ntwk *testcontainers.DockerNetwork,
	databaseURL string,
) error {
	containerRequest := testcontainers.ContainerRequest{
		Image: d.opts.ImageName,
		Cmd:   []string{"migrate", "sql", "up", "--read-from-env", "--yes"},
		Env: map[string]string{
			"LOG_LEVEL": "debug",
			"DSN":       databaseURL,
		},

		Files: []testcontainers.ContainerFile{
			{
				Reader:            strings.NewReader(d.configuration),
				ContainerFilePath: "/etc/config/keto.yml",
				FileMode:          definition.ContainerFileMode,
			},
		},
		WaitingFor: wait.ForExit(),
	}

	d.opts.Configure(ctx, ntwk, &containerRequest)

	ketoContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: containerRequest,

		Started: true,
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

func (d *ketoDependancy) Setup(ctx context.Context, ntwk *testcontainers.DockerNetwork) error {
	if len(d.opts.Dependencies) == 0 || !d.opts.Dependencies[0].GetInternalDS().IsDB() {
		return errors.New("no Database dependencies was supplied")
	}

	databaseURL := d.opts.Dependencies[0].GetInternalDS().String()
	err := d.migrateContainer(ctx, ntwk, databaseURL)
	if err != nil {
		return err
	}
	adminPort, err := nat.NewPort("tcp", d.opts.Ports[1])
	if err != nil {
		return err
	}

	containerRequest := testcontainers.ContainerRequest{
		Image: d.opts.ImageName,
		Cmd:   []string{"serve", "all", "--config", "/etc/config/keto.yml", "--dev"},
		Env: map[string]string{
			"LOG_LEVEL": "debug",
			"DSN":       databaseURL,
		},
		Files: []testcontainers.ContainerFile{
			{
				Reader:            strings.NewReader(d.configuration),
				ContainerFilePath: "/etc/config/keto.yml",
				FileMode:          definition.ContainerFileMode,
			},
		},
		WaitingFor: wait.ForHTTP("/health/ready").WithPort(adminPort),
	}

	d.opts.Configure(ctx, ntwk, &containerRequest)

	if !d.opts.UseHostMode {
		containerRequest.ExposedPorts = []string{fmt.Sprintf("%s/tcp", d.opts.Ports), "4466/tcp"}
	}

	ketoContainer, err := testcontainers.GenericContainer(ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: containerRequest,
			Started:          true,
		})

	if err != nil {
		return fmt.Errorf("failed to start ketoContainer: %w", err)
	}

	port, err := ketoContainer.MappedPort(ctx, adminPort)
	if err != nil {
		return fmt.Errorf("failed to get connection string for ketoContainer: %w", err)
	}

	host, err := ketoContainer.Host(ctx)
	if err != nil {
		return fmt.Errorf("failed to get connection string for ketoContainer: %w", err)
	}

	d.conn = frame.DataSource(fmt.Sprintf("http://%s", net.JoinHostPort(host, port.Port())))

	internalIP, err := ketoContainer.ContainerIP(ctx)
	if err != nil {
		return fmt.Errorf("failed to get internal host ip for ketoContainer: %w", err)
	}
	d.internalConn = frame.DataSource(fmt.Sprintf("http://%s", net.JoinHostPort(internalIP, adminPort.Port())))

	d.container = ketoContainer
	return nil
}

func (d *ketoDependancy) GetDS() frame.DataSource {
	return d.conn
}
func (d *ketoDependancy) GetInternalDS() frame.DataSource {
	return d.internalConn
}

func (d *ketoDependancy) GetRandomisedDS(
	_ context.Context,
	_ string,
) (frame.DataSource, func(context.Context), error) {
	return d.GetDS(), func(_ context.Context) {
	}, nil
}

func (d *ketoDependancy) Cleanup(ctx context.Context) {
	if d.container != nil {
		if err := d.container.Terminate(ctx); err != nil {
			log := util.Log(ctx)
			log.WithError(err).Error("Failed to terminate nats container")
		}
	}
}
