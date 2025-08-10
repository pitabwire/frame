package testoryhydra

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/docker/docker/api/types/container"
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
	// HydraPort is the default port for Hydra.
	HydraPort = "4445"

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
		Port:           HydraPort,
		UseHostMode:    false,
		DisableLogging: true,
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
	hydraContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:    d.opts.ImageName,
			Networks: []string{ntwk.Name},
			Cmd:      []string{"migrate", "sql", "up", "--read-from-env", "--yes"},
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
			HostConfigModifier: func(hostConfig *container.HostConfig) {
				if d.opts.UseHostMode {
					hostConfig.NetworkMode = "host"
				}
			},
			LogConsumerCfg: definition.LogConfig(ctx, d.opts.DisableLogging, d.opts.LoggingTimeout),
		},

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
	if len(d.opts.Dependancies) == 0 || !d.opts.Dependancies[0].GetDS().IsDB() {
		return errors.New("no Database dependencies was supplied")
	}

	databaseURL := d.opts.Dependancies[0].GetInternalDS().String()
	err := d.migrateContainer(ctx, ntwk, databaseURL)
	if err != nil {
		return err
	}

	hydraPort, err := nat.NewPort("tcp", d.opts.Port)
	if err != nil {
		return err
	}
	hydraContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:    d.opts.ImageName,
			Networks: []string{ntwk.Name},
			NetworkAliases: map[string][]string{
				ntwk.Name: {"hydra", "auth-hydra"},
			},
			ExposedPorts: []string{hydraPort.Port()},
			Cmd:          []string{"serve", "all", "--config", "/etc/config/hydra.yml", "--dev"},
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
			WaitingFor: wait.ForHTTP("/health/ready").WithPort(hydraPort),
			HostConfigModifier: func(hostConfig *container.HostConfig) {
				if d.opts.UseHostMode {
					hostConfig.NetworkMode = "host"
				}
			},
			LogConsumerCfg: definition.LogConfig(ctx, d.opts.DisableLogging, d.opts.LoggingTimeout),
		},
		Started: true,
	})

	if err != nil {
		return fmt.Errorf("failed to start hydraContainer: %w", err)
	}

	port, err := hydraContainer.MappedPort(ctx, hydraPort)
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
		fmt.Sprintf("http://%s", net.JoinHostPort(internalIP, d.opts.Port)),
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
