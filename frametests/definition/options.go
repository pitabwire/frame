package definition

import (
	"context"
	"strconv"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
)

const HostNetworkingMode = "host"

type ContainerOpts struct {
	ImageName string
	UserName  string
	Password  string

	Port           string
	UseHostMode    bool
	NetworkAliases []string

	Dependancies []DependancyConn

	EnableLogging  bool
	LoggingTimeout time.Duration
}

func (o *ContainerOpts) Setup(opts ...ContainerOption) {
	timeoutOpt := WithLoggingTimeout(DefaultLogProductionTimeout)
	timeoutOpt(o)

	for _, opt := range opts {
		opt(o)
	}
}

func (o *ContainerOpts) Configure(ctx context.Context, ntwk *testcontainers.DockerNetwork, containerRequest *testcontainers.ContainerRequest) {
	if o.EnableLogging {
		containerRequest.LogConsumerCfg = LogConfig(ctx, o.LoggingTimeout)
	}

	if o.UseHostMode {
		containerRequest.HostConfigModifier = func(hostConfig *container.HostConfig) {
			hostConfig.NetworkMode = "host"
		}
	} else {
		containerRequest.ExposedPorts = []string{o.Port}

		containerRequest.Networks = []string{ntwk.Name}
		containerRequest.NetworkAliases = map[string][]string{
			ntwk.Name: o.NetworkAliases,
		}
	}

}

func (o *ContainerOpts) ConfigurationExtend(ctx context.Context, ntwk *testcontainers.DockerNetwork, containerCustomize ...testcontainers.ContainerCustomizer) []testcontainers.ContainerCustomizer {

	if o.EnableLogging {
		containerCustomize = append(
			containerCustomize,
			testcontainers.WithLogConsumerConfig(LogConfig(ctx, o.LoggingTimeout)),
		)
	}

	if o.UseHostMode {
		containerCustomize = append(containerCustomize, testcontainers.WithHostConfigModifier(
			func(hostConfig *container.HostConfig) {
				hostConfig.NetworkMode = HostNetworkingMode
			}))
	} else {

		containerCustomize = append(containerCustomize,
			testcontainers.WithExposedPorts(o.Port),
			network.WithNetwork([]string{ntwk.Name}, ntwk),
			network.WithNetworkName(o.NetworkAliases, ntwk.Name))
	}

	return containerCustomize
}

// ContainerOption is a type that can be used to configure the container creation request.
type ContainerOption func(req *ContainerOpts)

// WithImageName allows to set the image name to use for testing.
func WithImageName(imageName string) ContainerOption {
	return func(original *ContainerOpts) {
		original.ImageName = imageName
	}
}

// WithUserName allows to set the user name to use for testing.
func WithUserName(userName string) ContainerOption {
	return func(original *ContainerOpts) {
		original.UserName = userName
	}
}

// WithPassword allows to set the password to use for testing.
func WithPassword(password string) ContainerOption {
	return func(original *ContainerOpts) {
		original.Password = password
	}
}

// WithPort allows to set the port to use for testing.
func WithPort(port int) ContainerOption {
	return func(original *ContainerOpts) {
		original.Port = strconv.Itoa(port)
	}
}

// WithUseHostMode allows to set the use host mode to use for testing.
func WithUseHostMode(useHostMode bool) ContainerOption {
	return func(original *ContainerOpts) {
		original.UseHostMode = useHostMode
	}
}

// WithNetworkAliases allows to set the network aliases to use for testing.
func WithNetworkAliases(networkAliases []string) ContainerOption {
	return func(original *ContainerOpts) {
		original.NetworkAliases = networkAliases
	}
}

// WithDisableLogging allows to set the disable logging to use for testing.
func WithDisableLogging(disableLogging bool) ContainerOption {
	return func(original *ContainerOpts) {
		original.EnableLogging = disableLogging
	}
}

// WithLoggingTimeout allows to set the logging timeout to use for testing.
func WithLoggingTimeout(loggingTimeout time.Duration) ContainerOption {
	return func(original *ContainerOpts) {
		original.LoggingTimeout = loggingTimeout
	}
}

// WithDependancies allows to set the dependancies to use for testing.
func WithDependancies(dependancies ...DependancyConn) ContainerOption {
	return func(original *ContainerOpts) {
		original.Dependancies = dependancies
	}
}
