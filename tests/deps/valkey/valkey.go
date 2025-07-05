package nats

import (
	"context"
	"fmt"
	"github.com/pitabwire/frame/tests/definitions"

	"github.com/pitabwire/util"
	tcValKey "github.com/testcontainers/testcontainers-go/modules/valkey"

	"github.com/pitabwire/frame"
)

const (

	// ValKey configuration.

	ValKeyImage = "docker.io/valkey/valkey:8.1"

	ValKeyUser    = "frame"
	ValKeyPass    = "fr@m3"
	ValKeyCluster = "frame_test"
)

type ValKeyDependancy struct {
	image    string
	username string
	password string
	cluster  string

	conn frame.DataSource

	container *tcValKey.ValkeyContainer
}

func NewValKeyDep() definitions.Dependancy {
	return NewValKeyDepWithCred(ValKeyImage, ValKeyUser, ValKeyPass, ValKeyCluster)
}

func NewValKeyDepWithCred(natsImage, natsUserName, natsPassword, cluster string) definitions.Dependancy {
	return &ValKeyDependancy{
		image:    natsImage,
		username: natsUserName,
		password: natsPassword,
		cluster:  cluster,
	}
}
func (vkd *ValKeyDependancy) Setup(ctx context.Context) error {
	container, err := tcValKey.Run(ctx, vkd.image)
	if err != nil {
		return fmt.Errorf("failed to start nats container: %w", err)
	}
	conn, err := container.ConnectionString(ctx)
	if err != nil {
		return fmt.Errorf("failed to get connection string for postgres container: %w", err)
	}

	vkd.conn = frame.DataSource(conn)

	vkd.container = container
	return nil
}

func (vkd *ValKeyDependancy) GetDS() frame.DataSource {
	return vkd.conn
}

func (vkd *ValKeyDependancy) GetPrefixedDS(
	_ context.Context,
	_ string,
) (frame.DataSource, func(context.Context), error) {
	return vkd.conn, func(_ context.Context) {
	}, nil
}

func (vkd *ValKeyDependancy) Cleanup(ctx context.Context) {
	if vkd.container != nil {
		if err := vkd.container.Terminate(ctx); err != nil {
			log := util.Log(ctx)
			log.WithError(err).Error("Failed to terminate valkey container")
		}
	}
}
