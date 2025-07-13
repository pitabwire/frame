package testoryketo

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/pitabwire/util"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/tests/testdef"
)

const (
	OryKetoImage = "oryd/keto:latest"

	KetoConfiguration = `
## ORY Keto Configuration
#

## serve ##
#
serve:
  ## Write API (http and gRPC) ##
  #
  write:
    host: 0.0.0.0


  ## Read API (http and gRPC) ##
  #
  read:
    host: 0.0.0.0
log:
  level: info
namespaces:
  - id: 0
    name: default

`
)

type ketoDependancy struct {
	image              string
	configuration      string
	databaseConnection string

	conn         frame.DataSource
	internalConn frame.DataSource

	container testcontainers.Container
}

func New() testdef.TestResource {
	return NewWithCred(OryKetoImage, KetoConfiguration, "")
}

func NewWithCred(image, configuration, databaseConnection string) testdef.TestResource {
	return &ketoDependancy{
		image:              image,
		configuration:      configuration,
		databaseConnection: databaseConnection,
	}
}

func (d *ketoDependancy) Container() testcontainers.Container {
	return d.container
}

func (d *ketoDependancy) migrateContainer(ctx context.Context, ntwk *testcontainers.DockerNetwork) error {
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:    OryKetoImage,
			Networks: []string{ntwk.Name},
			Cmd:      []string{"migrate", "sql", "up", "--read-from-env", "--yes"},
			Env: map[string]string{
				"LOG_LEVEL": "debug",
				"DSN":       d.databaseConnection,
			},

			Files: []testcontainers.ContainerFile{
				{
					Reader:            strings.NewReader(d.configuration),
					ContainerFilePath: "/etc/config/keto.yml",
					FileMode:          testdef.ContainerFileMode,
				},
			},
			WaitingFor: wait.ForExit(),
		},

		Started: true,
	})
	if err != nil {
		return err
	}

	err = container.Terminate(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (d *ketoDependancy) Setup(ctx context.Context, ntwk *testcontainers.DockerNetwork) error {
	err := d.migrateContainer(ctx, ntwk)
	if err != nil {
		return err
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        OryKetoImage,
			Networks:     []string{ntwk.Name},
			ExposedPorts: []string{"4466/tcp", "4467/tcp"},
			Cmd:          []string{"serve", "all", "--config", "/etc/config/keto.yml", "--dev"},
			Env: map[string]string{
				"LOG_LEVEL": "debug",
				"DSN":       d.databaseConnection,
			},
			Files: []testcontainers.ContainerFile{
				{
					Reader:            strings.NewReader(d.configuration),
					ContainerFilePath: "/etc/config/keto.yml",
					FileMode:          testdef.ContainerFileMode,
				},
			},
			WaitingFor: wait.ForHTTP("/health/ready").WithPort("4445/tcp"),
		},
		Started: true,
	})

	if err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	port, err := container.MappedPort(ctx, "4467/tcp")
	if err != nil {
		return fmt.Errorf("failed to get connection string for container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		return fmt.Errorf("failed to get connection string for container: %w", err)
	}

	d.conn = frame.DataSource(fmt.Sprintf("http://%s", net.JoinHostPort(host, port.Port())))

	internalIP, err := container.ContainerIP(ctx)
	if err != nil {
		return fmt.Errorf("failed to get internal host ip for container: %w", err)
	}
	d.internalConn = frame.DataSource(fmt.Sprintf("http://%s", net.JoinHostPort(internalIP, "4445")))

	d.container = container
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
