package definitions

import (
	"context"

	"github.com/pitabwire/frame"
)

type Dependancy interface {
	Setup(ctx context.Context) error
	GetDS() frame.DataSource
	GetPrefixedDS(ctx context.Context, randomisedPrefix string) (frame.DataSource, func(context.Context), error)
	Cleanup(ctx context.Context)
}

type DependancyOption struct {
	name   string
	prefix string
	deps   []Dependancy
}

func (opt *DependancyOption) Name() string {
	return opt.name
}
func (opt *DependancyOption) Database() Dependancy {
	for _, dep := range opt.deps {
		if dep.GetDS().IsDB() {
			return dep
		}
	}
	return nil
}
func (opt *DependancyOption) Cache() Dependancy {
	for _, dep := range opt.deps {
		if dep.GetDS().IsCache() {
			return dep
		}
	}
	return nil
}
func (opt *DependancyOption) Queue() Dependancy {
	for _, dep := range opt.deps {
		if dep.GetDS().IsQueue() {
			return dep
		}
	}
	return nil
}
