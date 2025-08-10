package definition

import (
	"strconv"
	"time"
)

const HostNetworkingMode = "host"

type ContainerOpts struct {
	ImageName string
	UserName  string
	Password  string

	Port        string
	UseHostMode bool

	Dependancies []DependancyConn

	DisableLogging bool
	LoggingTimeout time.Duration
}

func (o *ContainerOpts) Setup(opts ...ContainerOption) {
	timeoutOpt := WithLoggingTimeout(DefaultLogProductionTimeout)
	timeoutOpt(o)

	for _, opt := range opts {
		opt(o)
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

// WithDisableLogging allows to set the disable logging to use for testing.
func WithDisableLogging(disableLogging bool) ContainerOption {
	return func(original *ContainerOpts) {
		original.DisableLogging = disableLogging
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
