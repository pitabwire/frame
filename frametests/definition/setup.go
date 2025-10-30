package definition

import (
	"context"

	"github.com/testcontainers/testcontainers-go"

	"github.com/pitabwire/frame/data"
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
	GetDS(ctx context.Context) data.DSN
	GetInternalDS(ctx context.Context) data.DSN
	GetRandomisedDS(ctx context.Context, randomisedPrefix string) (data.DSN, func(context.Context), error)
}

type TestResource interface {
	DependancyRes
	DependancyConn
}

type DependencyOption struct {
	name   string
	prefix string
	deps   []DependancyConn
}

func NewDependancyOption(name string, prefix string, deps []DependancyConn) *DependencyOption {
	return &DependencyOption{
		name:   name,
		prefix: prefix,
		deps:   deps,
	}
}

func (opt *DependencyOption) Name() string {
	return opt.name
}
func (opt *DependencyOption) Prefix() string {
	return opt.prefix
}
func (opt *DependencyOption) All() []DependancyConn {
	return opt.deps
}

func (opt *DependencyOption) ByImageName(imageName string) DependancyConn {
	for _, dep := range opt.deps {
		if dep.Name() == imageName {
			return dep
		}
	}
	return nil
}

func (opt *DependencyOption) ByIsDatabase(ctx context.Context) DependancyConn {
	for _, dep := range opt.deps {
		if dep.GetDS(ctx).IsDB() {
			return dep
		}
	}
	return nil
}

func (opt *DependencyOption) ByIsCache(ctx context.Context) DependancyConn {
	for _, dep := range opt.deps {
		if dep.GetDS(ctx).IsCache() {
			return dep
		}
	}
	return nil
}

func (opt *DependencyOption) ByIsQueue(ctx context.Context) DependancyConn {
	for _, dep := range opt.deps {
		if dep.GetDS(ctx).IsQueue() {
			return dep
		}
	}
	return nil
}
