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

func (opt *DependancyOption) Database(ctx context.Context) []DependancyConn {
	var deps []DependancyConn
	for _, dep := range opt.deps {
		if dep.GetDS(ctx).IsDB() {
			deps = append(deps, dep)
		}
	}
	return deps
}
func (opt *DependancyOption) Cache(ctx context.Context) []DependancyConn {
	var deps []DependancyConn
	for _, dep := range opt.deps {
		if dep.GetDS(ctx).IsCache() {
			deps = append(deps, dep)
		}
	}
	return deps
}
func (opt *DependancyOption) Queue(ctx context.Context) []DependancyConn {
	var deps []DependancyConn
	for _, dep := range opt.deps {
		if dep.GetDS(ctx).IsQueue() {
			deps = append(deps, dep)
		}
	}
	return deps
}
