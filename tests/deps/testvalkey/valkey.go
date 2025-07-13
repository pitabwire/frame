package nats

import (
	"context"
	"fmt"
	"net"

	"github.com/pitabwire/util"
	"github.com/testcontainers/testcontainers-go"
	tcValKey "github.com/testcontainers/testcontainers-go/modules/valkey"
	"github.com/testcontainers/testcontainers-go/network"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/tests/testdef"
)

const (

	// ValKey configuration.

	ValKeyImage = "docker.io/valkey/valkey:8.1"

	ValKeyUser    = "frame"
	ValKeyPass    = "fr@m3"
	ValKeyCluster = "frame_test"
)

type valKeyDependancy struct {
	image    string
	username string
	password string
	cluster  string

	conn         frame.DataSource
	internalConn frame.DataSource

	container *tcValKey.ValkeyContainer
}

func NewValKeyDep() testdef.TestResource {
	return NewValKeyDepWithCred(ValKeyImage, ValKeyUser, ValKeyPass, ValKeyCluster)
}

func NewValKeyDepWithCred(natsImage, natsUserName, natsPassword, cluster string) testdef.TestResource {
	return &valKeyDependancy{
		image:    natsImage,
		username: natsUserName,
		password: natsPassword,
		cluster:  cluster,
	}
}
func (vkd *valKeyDependancy) Setup(ctx context.Context, ntwk *testcontainers.DockerNetwork) error {
	container, err := tcValKey.Run(ctx, vkd.image, network.WithNetwork([]string{ntwk.Name}, ntwk))
	if err != nil {
		return fmt.Errorf("failed to start nats container: %w", err)
	}

	conn, err := container.ConnectionString(ctx)
	if err != nil {
		return fmt.Errorf("failed to get connection string for container: %w", err)
	}

	vkd.conn = frame.DataSource(conn)

	internalIP, err := container.ContainerIP(ctx)
	if err != nil {
		return fmt.Errorf("failed to get internal host ip for container: %w", err)
	}
	vkd.internalConn = frame.DataSource(fmt.Sprintf("redis://%s", net.JoinHostPort(internalIP, "6379")))
	vkd.container = container
	return nil
}

func (vkd *valKeyDependancy) GetDS() frame.DataSource {
	return vkd.conn
}

func (vkd *valKeyDependancy) GetInternalDS() frame.DataSource {
	return vkd.conn
}

func (vkd *valKeyDependancy) GetRandomisedDS(
	_ context.Context,
	_ string,
) (frame.DataSource, func(context.Context), error) {
	return vkd.GetDS(), func(_ context.Context) {
	}, nil
}

func (vkd *valKeyDependancy) Cleanup(ctx context.Context) {
	if vkd.container != nil {
		if err := vkd.container.Terminate(ctx); err != nil {
			log := util.Log(ctx)
			log.WithError(err).Error("Failed to terminate valkey container")
		}
	}
}
