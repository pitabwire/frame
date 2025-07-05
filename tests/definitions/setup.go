package definitions

import (
	"context"

	"github.com/pitabwire/frame"
)

type DependancyRes interface {
	Setup(ctx context.Context) error
	Cleanup(ctx context.Context)
}

type DependancyConn interface {
	GetDS() frame.DataSource
	GetPrefixedDS(ctx context.Context, randomisedPrefix string) (frame.DataSource, func(context.Context), error)
}

type TestResource interface {
	DependancyRes
	DependancyConn
}

type DependancyOption struct {
	name   string
	prefix string
	deps   []TestResource
}

func NewDependancyOption(name string, prefix string, deps []TestResource) *DependancyOption {
	return &DependancyOption{
		name:   name,
		prefix: prefix,
		deps:   deps,
	}
}

func (opt *DependancyOption) Name() string {
	return opt.name
}
func (opt *DependancyOption) Database() DependancyConn {
	for _, dep := range opt.deps {
		if dep.GetDS().IsDB() {
			return dep
		}
	}
	return nil
}
func (opt *DependancyOption) Cache() DependancyConn {
	for _, dep := range opt.deps {
		if dep.GetDS().IsCache() {
			return dep
		}
	}
	return nil
}
func (opt *DependancyOption) Queue() DependancyConn {
	for _, dep := range opt.deps {
		if dep.GetDS().IsQueue() {
			return dep
		}
	}
	return nil
}
