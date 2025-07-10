package testdef

import (
	"context"

	"github.com/testcontainers/testcontainers-go"

	"github.com/pitabwire/frame"
)

type DependancyRes interface {
	Setup(ctx context.Context, network *testcontainers.DockerNetwork) error
	Cleanup(ctx context.Context)
}

type DependancyConn interface {
	GetDS() frame.DataSource
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
func (opt *DependancyOption) Database() []DependancyConn {
	var deps []DependancyConn
	for _, dep := range opt.deps {
		if dep.GetDS().IsDB() {
			deps = append(deps, dep)
		}
	}
	return deps
}
func (opt *DependancyOption) Cache() []DependancyConn {
	var deps []DependancyConn
	for _, dep := range opt.deps {
		if dep.GetDS().IsCache() {
			deps = append(deps, dep)
		}
	}
	return deps
}
func (opt *DependancyOption) Queue() []DependancyConn {
	var deps []DependancyConn
	for _, dep := range opt.deps {
		if dep.GetDS().IsQueue() {
			deps = append(deps, dep)
		}
	}
	return deps
}
