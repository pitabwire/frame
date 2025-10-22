package definition

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/docker/go-connections/nat"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/util"
	"github.com/testcontainers/testcontainers-go"
)

type DefaultImpl struct {
	opts      ContainerOpts
	container testcontainers.Container

	DefaultScheme string
	DefaultPort   nat.Port
}

func NewDefaultImpl(opts ContainerOpts, scheme string, containerOpts ...ContainerOption) *DefaultImpl {
	opts.Setup(containerOpts...)

	defaultImpl := DefaultImpl{
		opts:          opts,
		DefaultScheme: scheme,
		DefaultPort:   nat.Port(opts.Ports[0]),
	}

	return &defaultImpl
}

func (d *DefaultImpl) Name() string {
	return d.opts.ImageName
}

func (d *DefaultImpl) Opts() *ContainerOpts {
	return &d.opts
}

func (d *DefaultImpl) Configure(
	ctx context.Context,
	ntwk *testcontainers.DockerNetwork,
	containerRequest *testcontainers.ContainerRequest,
) {
	d.opts.Configure(ctx, ntwk, containerRequest)
}

func (d *DefaultImpl) ConfigurationExtend(
	ctx context.Context,
	ntwk *testcontainers.DockerNetwork,
	containerCustomize ...testcontainers.ContainerCustomizer,
) []testcontainers.ContainerCustomizer {
	return d.opts.ConfigurationExtend(ctx, ntwk, containerCustomize...)
}

func (d *DefaultImpl) Container() testcontainers.Container {
	return d.container
}

func (d *DefaultImpl) SetContainer(container testcontainers.Container) {
	d.container = container
}

func (d *DefaultImpl) PortMapping(ctx context.Context, port string) (string, error) {
	mappedPort, err := d.container.MappedPort(ctx, nat.Port(port))
	if err != nil {
		return "", err
	}
	return mappedPort.Port(), nil
}

func (d *DefaultImpl) Endpoint(ctx context.Context, scheme string, port string) (frame.DataSource, error) {
	conn, err := d.container.PortEndpoint(ctx, nat.Port(port), scheme)
	if err != nil {
		return "", err
	}

	conn = strings.Replace(conn, "localhost", "127.0.0.1", 1)

	return frame.DataSource(conn), nil
}

func (d *DefaultImpl) InternalEndpoint(ctx context.Context, scheme string, port string) (frame.DataSource, error) {
	internalIP, err := d.container.ContainerIP(ctx)
	if err != nil {
		return "", err
	}

	if internalIP == "" && d.opts.UseHostMode {
		internalIP, err = d.container.Host(ctx)
		if err != nil {
			return "", err
		}
	}

	if internalIP == "localhost" {
		internalIP = "127.0.0.1"
	}

	hostPort := net.JoinHostPort(internalIP, port)
	if scheme == "" {
		return frame.DataSource(hostPort), nil
	}
	return frame.DataSource(fmt.Sprintf("%s://%s", scheme, hostPort)), nil
}

func (d *DefaultImpl) GetDS(ctx context.Context) frame.DataSource {
	ds, err := d.Endpoint(ctx, d.DefaultScheme, d.DefaultPort.Port())
	if err != nil {
		logger := util.Log(ctx).WithField("image", d.opts.ImageName)
		logger.WithError(err).Error("failed to get default connection for Container")
	}

	return ds
}

func (d *DefaultImpl) GetInternalDS(ctx context.Context) frame.DataSource {
	ds, err := d.InternalEndpoint(ctx, d.DefaultScheme, d.DefaultPort.Port())
	if err != nil {
		logger := util.Log(ctx).WithField("image", d.opts.ImageName)
		logger.WithError(err).Error("failed to get default internal connection for Container")
	}

	return ds
}

func (d *DefaultImpl) GetRandomisedDS(
	ctx context.Context,
	_ string,
) (frame.DataSource, func(context.Context), error) {
	return d.GetDS(ctx), func(_ context.Context) {
	}, nil
}

func (d *DefaultImpl) Cleanup(ctx context.Context) {
	if d.container != nil {
		if err := d.container.Terminate(ctx); err != nil {
			log := util.Log(ctx)
			log.WithField("image", d.opts.ImageName).WithError(err).Info("Container termination was had and error")
		}
	}
}
