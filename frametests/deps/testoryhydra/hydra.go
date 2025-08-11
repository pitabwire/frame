package testoryhydra

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
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

type dependancy struct {
	opts          definition.ContainerOpts
	configuration string

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

	return &dependancy{
		opts:          opts,
		configuration: configuration,
	}
}

func (d *dependancy) Name() string {
	return d.opts.ImageName
}

func (d *dependancy) Container() testcontainers.Container {
	return d.container
}

func (d *dependancy) migrateContainer(
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

func (d *dependancy) Setup(ctx context.Context, ntwk *testcontainers.DockerNetwork) error {
	if len(d.opts.Dependencies) == 0 || !d.opts.Dependencies[0].GetDS(ctx).IsDB() {
		return errors.New("no Database dependencies was supplied")
	}

	databaseURL := d.opts.Dependencies[0].GetInternalDS(ctx).String()
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

	d.container = hydraContainer
	return nil
}

func (d *dependancy) GetDS(ctx context.Context) frame.DataSource {
	port := nat.Port(d.opts.Ports[1])
	conn, err := d.container.PortEndpoint(ctx, port, "http")
	if err != nil {
		logger := util.Log(ctx).WithField("image", d.opts.ImageName)
		logger.WithError(err).Error("failed to get connection for Container")
	}

	return frame.DataSource(conn)
}

func (d *dependancy) GetInternalDS(ctx context.Context) frame.DataSource {
	internalIP, err := d.container.ContainerIP(ctx)
	if err != nil {
		logger := util.Log(ctx).WithField("image", d.opts.ImageName)
		logger.WithError(err).Error("failed to get internal host ip for Container")
		return ""
	}

	if internalIP == "" && d.opts.UseHostMode {
		internalIP, err = d.container.Host(ctx)
		if err != nil {
			logger := util.Log(ctx).WithField("image", d.opts.ImageName)
			logger.WithError(err).Error("failed to get host ip for Container")
			return ""
		}
	}
	port := nat.Port(d.opts.Ports[1])

	return frame.DataSource(fmt.Sprintf("http://%s", net.JoinHostPort(internalIP, strconv.Itoa(port.Int()))))
}

func (d *dependancy) GetRandomisedDS(
	ctx context.Context,
	_ string,
) (frame.DataSource, func(context.Context), error) {
	return d.GetDS(ctx), func(_ context.Context) {
	}, nil
}

func (d *dependancy) Cleanup(ctx context.Context) {
	if d.container != nil {
		if err := d.container.Terminate(ctx); err != nil {
			log := util.Log(ctx)
			log.WithError(err).Error("Failed to terminate nats container")
		}
	}
}
