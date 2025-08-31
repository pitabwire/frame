package definition

import (
	"context"

	"github.com/testcontainers/testcontainers-go"

	"github.com/pitabwire/frame"
)

const ContainerFileMode = 0o755

type DependancyRes interface {
	Name() string
	Setup(ctx context.Context, network *testcontainers.DockerNetwork) error
	Cleanup(ctx context.Context)
	Container() testcontainers.Container
}

type DependancyConn interface {
	Name() string
	PortMapping(ctx context.Context, port string) (string, error)
	GetDS(ctx context.Context) frame.DataSource
	GetInternalDS(ctx context.Context) frame.DataSource
	GetRandomisedDS(ctx context.Context, randomisedPrefix string) (frame.DataSource, func(context.Context), error)
}

type TestResource interface {
	DependancyRes
	DependancyConn
}

type DependancyOption struct {
	name   string
	prefix string
	deps   []DependancyConn
}

func NewDependancyOption(name string, prefix string, deps []DependancyConn) *DependancyOption {
	return &DependancyOption{
		name:   name,
		prefix: prefix,
		deps:   deps,
	}
}

func (opt *DependancyOption) Name() string {
	return opt.name
}
func (opt *DependancyOption) Prefix() string {
	return opt.prefix
}
func (opt *DependancyOption) All() []DependancyConn {
	return opt.deps
}

func (opt *DependancyOption) ByImageName(imageName string) DependancyConn {
	for _, dep := range opt.deps {
		if dep.Name() == imageName {
			return dep
		}
	}
	return nil
}

func (opt *DependancyOption) ByIsDatabase(ctx context.Context) DependancyConn {
	for _, dep := range opt.deps {
		if dep.GetDS(ctx).IsDB() {
			return dep
		}
	}
	return nil
}

func (opt *DependancyOption) ByIsCache(ctx context.Context) DependancyConn {
	for _, dep := range opt.deps {
		if dep.GetDS(ctx).IsCache() {
			return dep
		}
	}
	return nil
}

func (opt *DependancyOption) ByIsQueue(ctx context.Context) DependancyConn {
	for _, dep := range opt.deps {
		if dep.GetDS(ctx).IsQueue() {
			return dep
		}
	}
	return nil
}
