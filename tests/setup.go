package tests

import (
	"context"
	"testing"

	"github.com/pitabwire/frame"
)

type Dependancy interface {
	Setup(ctx context.Context) error
	GetDS() frame.DataSource
	GetPrefixedDS(ctx context.Context, randomisedPrefix string) (frame.DataSource, func(context.Context), error)
	Cleanup(ctx context.Context)
}

type DependancyOption struct {
	name string
	deps []Dependancy
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

// WithTestDependancies Creates subtests with each known DependancyOption.
func WithTestDependancies(t *testing.T, options []DependancyOption, testFn func(t *testing.T, db DependancyOption)) {
	for _, opt := range options {
		t.Run(opt.Name(), func(tt *testing.T) {
			// Removed tt.Parallel() as it conflicts with t.Setenv() used in GetService
			testFn(tt, opt)
		})
	}
}
