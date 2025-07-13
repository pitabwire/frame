package testoryhydra

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

	// NATS configuration.

	OryHydraImage = "oryd/hydra:latest"

	HydraConfiguration = `
## ORY Hydra Configuration
#

serve:
  admin:
    host: 0.0.0.0
  public:
    host: 0.0.0.0
  cookies:
    same_site_mode: Lax

urls:
  self:
    issuer: http://127.0.0.1:4444
  consent: http://127.0.0.1:3000/consent
  login: http://127.0.0.1:3000/login
  logout: http://127.0.0.1:3000/logout

secrets:
  system:
    - youReallyNeedToChangeThis

oidc:
  subject_identifiers:
    supported_types:
      - public

strategies:
  access_token: jwt

`
)

type hydraDependancy struct {
	image              string
	configuration      string
	databaseConnection string

	conn         frame.DataSource
	internalConn frame.DataSource

	container testcontainers.Container
}

func New() testdef.TestResource {
	return NewWithCred(OryHydraImage, HydraConfiguration, "")
}

func NewWithCred(image, configuration, databaseConnection string) testdef.TestResource {
	return &hydraDependancy{
		image:              image,
		configuration:      configuration,
		databaseConnection: databaseConnection,
	}
}

func (d *hydraDependancy) Container() testcontainers.Container {
	return d.container
}

func (d *hydraDependancy) migrateContainer(ctx context.Context, ntwk *testcontainers.DockerNetwork) error {
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:    OryHydraImage,
			Networks: []string{ntwk.Name},
			Cmd:      []string{"migrate", "sql", "up", "--read-from-env", "--yes"},
			Env: map[string]string{
				"LOG_LEVEL": "debug",
				"DSN":       d.databaseConnection,
			},

			Files: []testcontainers.ContainerFile{
				{
					Reader:            strings.NewReader(d.configuration),
					ContainerFilePath: "/etc/config/hydra.yml",
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

func (d *hydraDependancy) Setup(ctx context.Context, ntwk *testcontainers.DockerNetwork) error {
	err := d.migrateContainer(ctx, ntwk)
	if err != nil {
		return err
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        OryHydraImage,
			Networks:     []string{ntwk.Name},
			ExposedPorts: []string{"4444/tcp", "4445/tcp"},
			Cmd:          []string{"serve", "all", "--config", "/etc/config/hydra.yml", "--dev"},
			Env: map[string]string{
				"LOG_LEVEL": "debug",
				"DSN":       d.databaseConnection,
			},
			Files: []testcontainers.ContainerFile{
				{
					Reader:            strings.NewReader(d.configuration),
					ContainerFilePath: "/etc/config/hydra.yml",
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

	port, err := container.MappedPort(ctx, "4445/tcp")
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

func (d *hydraDependancy) GetDS() frame.DataSource {
	return d.conn
}
func (d *hydraDependancy) GetInternalDS() frame.DataSource {
	return d.internalConn
}

func (d *hydraDependancy) GetRandomisedDS(
	_ context.Context,
	_ string,
) (frame.DataSource, func(context.Context), error) {
	return d.GetDS(), func(_ context.Context) {
	}, nil
}

func (d *hydraDependancy) Cleanup(ctx context.Context) {
	if d.container != nil {
		if err := d.container.Terminate(ctx); err != nil {
			log := util.Log(ctx)
			log.WithError(err).Error("Failed to terminate nats container")
		}
	}
}
