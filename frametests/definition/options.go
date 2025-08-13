package definition

import (
	"context"
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

	Ports []string

	UseHostMode    bool
	NetworkAliases []string

	Environment  map[string]string
	Dependencies []DependancyConn

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

func (o *ContainerOpts) Configure(
	ctx context.Context,
	ntwk *testcontainers.DockerNetwork,
	containerRequest *testcontainers.ContainerRequest,
) {
	if o.EnableLogging {
		containerRequest.LogConsumerCfg = LogConfig(ctx, o.LoggingTimeout)
	}

	if o.UseHostMode {
		containerRequest.ExposedPorts = nil
		containerRequest.HostConfigModifier = func(hostConfig *container.HostConfig) {
			hostConfig.NetworkMode = "host"
		}
	} else {
		containerRequest.ExposedPorts = o.Ports
		containerRequest.Networks = []string{ntwk.Name}
		containerRequest.NetworkAliases = map[string][]string{
			ntwk.Name: o.NetworkAliases,
		}
	}
}

func (o *ContainerOpts) ConfigurationExtend(
	ctx context.Context,
	ntwk *testcontainers.DockerNetwork,
	containerCustomize ...testcontainers.ContainerCustomizer,
) []testcontainers.ContainerCustomizer {
	if o.EnableLogging {
		containerCustomize = append(
			containerCustomize,
			testcontainers.WithLogConsumerConfig(LogConfig(ctx, o.LoggingTimeout)),
		)
	}

	if o.UseHostMode {
		containerCustomize = append(containerCustomize,
			testcontainers.WithHostConfigModifier(
				func(hostConfig *container.HostConfig) {
					hostConfig.NetworkMode = HostNetworkingMode
				}))
	} else {
		containerCustomize = append(containerCustomize,
			withExposePorts(o.Ports...),
			network.WithNetwork([]string{ntwk.Name}, ntwk),
			network.WithNetworkName(o.NetworkAliases, ntwk.Name))
	}

	return containerCustomize
}

func (o *ContainerOpts) Env(defaultMap map[string]string) map[string]string {
	for k, val := range o.Environment {
		defaultMap[k] = val
	}
	return defaultMap
}

// withExposePorts appends the ports to the exposed ports for a container.
func withExposePorts(ports ...string) testcontainers.CustomizeRequestOption {
	return func(req *testcontainers.GenericContainerRequest) error {
		req.ExposedPorts = ports
		return nil
	}
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

// WithPorts allows to set the ports to use for testing.
func WithPorts(ports ...string) ContainerOption {
	return func(original *ContainerOpts) {
		original.Ports = ports
	}
}

// WithEnvironment allows to set the environment to use for testing.
func WithEnvironment(environment map[string]string) ContainerOption {
	return func(original *ContainerOpts) {
		original.Environment = environment
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

// WithEnableLogging allows to enable logging to use for testing.
func WithEnableLogging(enableLogging bool) ContainerOption {
	return func(original *ContainerOpts) {
		original.EnableLogging = enableLogging
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
		original.Dependencies = dependancies
	}
}
