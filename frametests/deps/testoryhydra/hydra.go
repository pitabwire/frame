package testoryhydra

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
	// OryHydraImage is the Ory Hydra Image.
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
	opts          definition.ContainerOpts
	configuration string

	conn         frame.DataSource
	internalConn frame.DataSource

	container testcontainers.Container
}

func New() definition.TestResource {
	return NewWithOpts(HydraConfiguration)
}

func NewWithOpts(configuration string, containerOpts ...definition.ContainerOption) definition.TestResource {
	opts := definition.ContainerOpts{
		ImageName:      OryHydraImage,
		Ports:          []string{"4444", "4445"},
		NetworkAliases: []string{"hydra", "auth-hydra"},
	}
	opts.Setup(containerOpts...)

	return &hydraDependancy{
		opts:          opts,
		configuration: configuration,
	}
}

func (d *hydraDependancy) Name() string {
	return d.opts.ImageName
}

func (d *hydraDependancy) Container() testcontainers.Container {
	return d.container
}

func (d *hydraDependancy) migrateContainer(
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
				ContainerFilePath: "/etc/config/hydra.yml",
				FileMode:          definition.ContainerFileMode,
			},
		},
		WaitingFor: wait.ForExit(),
	}

	d.opts.Configure(ctx, ntwk, &containerRequest)

	hydraContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: containerRequest,

		Started: true,
	})
	if err != nil {
		return err
	}

	err = hydraContainer.Terminate(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (d *hydraDependancy) Setup(ctx context.Context, ntwk *testcontainers.DockerNetwork) error {
	if len(d.opts.Dependencies) == 0 || !d.opts.Dependencies[0].GetDS().IsDB() {
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
		Cmd:   []string{"serve", "all", "--config", "/etc/config/hydra.yml", "--dev"},
		Env: map[string]string{
			"LOG_LEVEL": "debug",
			"DSN":       databaseURL,
		},
		Files: []testcontainers.ContainerFile{
			{
				Reader:            strings.NewReader(d.configuration),
				ContainerFilePath: "/etc/config/hydra.yml",
				FileMode:          definition.ContainerFileMode,
			},
		},
		WaitingFor: wait.ForHTTP("/health/ready").WithPort(adminPort),
	}

	d.opts.Configure(ctx, ntwk, &containerRequest)

	hydraContainer, err := testcontainers.GenericContainer(ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: containerRequest,
			Started:          true,
		})

	if err != nil {
		return fmt.Errorf("failed to start hydraContainer: %w", err)
	}

	port, err := hydraContainer.MappedPort(ctx, adminPort)
	if err != nil {
		return fmt.Errorf("failed to get connection string for hydraContainer: %w", err)
	}

	host, err := hydraContainer.Host(ctx)
	if err != nil {
		return fmt.Errorf("failed to get connection string for hydraContainer: %w", err)
	}

	d.conn = frame.DataSource(fmt.Sprintf("http://%s", net.JoinHostPort(host, port.Port())))

	internalIP, err := hydraContainer.ContainerIP(ctx)
	if err != nil {
		return fmt.Errorf("failed to get internal host ip for hydraContainer: %w", err)
	}
	d.internalConn = frame.DataSource(
		fmt.Sprintf("http://%s", net.JoinHostPort(internalIP, adminPort.Port())),
	)

	d.container = hydraContainer
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
