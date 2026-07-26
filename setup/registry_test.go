package setup_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pitabwire/frame/v2/setup"
	"github.com/stretchr/testify/require"
)

func TestRegistryRunBulkOrder(t *testing.T) {
	t.Parallel()

	reg := setup.NewRegistry()
	order := make([]string, 0, 3)
	reg.RegisterFunc("migrate", func(context.Context) error {
		order = append(order, "migrate")
		return nil
	})
	reg.RegisterFunc("permissions", func(context.Context) error {
		order = append(order, "permissions")
		return nil
	})
	reg.RegisterFunc("bootstrap", func(context.Context) error {
		order = append(order, "bootstrap")
		return nil
	})

	require.NoError(t, reg.Run(context.Background(), "migrate", "bootstrap", "permissions"))
	require.Equal(t, []string{"migrate", "bootstrap", "permissions"}, order)
}

func TestRegistryRunAllUsesRegistrationOrder(t *testing.T) {
	t.Parallel()

	reg := setup.NewRegistry()
	order := make([]string, 0, 2)
	reg.Register(setup.Func{StepName: "a", Fn: func(context.Context) error {
		order = append(order, "a")
		return nil
	}})
	reg.Register(setup.Func{StepName: "b", Fn: func(context.Context) error {
		order = append(order, "b")
		return nil
	}})

	require.NoError(t, reg.RunAll(context.Background()))
	require.Equal(t, []string{"a", "b"}, order)
}

func TestRegistryReplaceKeepsOrder(t *testing.T) {
	t.Parallel()

	reg := setup.NewRegistry()
	var which string
	reg.RegisterFunc("migrate", func(context.Context) error {
		which = "first"
		return nil
	})
	reg.RegisterFunc("migrate", func(context.Context) error {
		which = "second"
		return nil
	})
	require.Equal(t, []string{"migrate"}, reg.Names())
	require.NoError(t, reg.RunAll(context.Background()))
	require.Equal(t, "second", which)
}

func TestRegistryUnknownAndEmpty(t *testing.T) {
	t.Parallel()

	reg := setup.NewRegistry()
	err := reg.Run(context.Background(), "nope")
	require.ErrorIs(t, err, setup.ErrUnknownStep)

	require.ErrorIs(t, setup.NewRegistry().RunAll(context.Background()), setup.ErrEmptyPlan)
}

func TestRegistryFailClosed(t *testing.T) {
	t.Parallel()

	reg := setup.NewRegistry()
	ranSecond := false
	reg.RegisterFunc("migrate", func(context.Context) error {
		return errors.New("schema boom")
	})
	reg.RegisterFunc("permissions", func(context.Context) error {
		ranSecond = true
		return nil
	})
	err := reg.Run(context.Background(), "migrate", "permissions")
	require.ErrorContains(t, err, "setup step \"migrate\"")
	require.False(t, ranSecond)
}

func TestSelect(t *testing.T) {
	t.Parallel()

	t.Run("argv setup with names", func(t *testing.T) {
		t.Parallel()
		sel := setup.Select([]string{"setup", "migrate", "permissions"}, false, "")
		require.True(t, sel.Active)
		require.Equal(t, []string{"migrate", "permissions"}, sel.Names)
	})

	t.Run("bare setup means all", func(t *testing.T) {
		t.Parallel()
		sel := setup.Select([]string{"setup"}, false, "")
		require.True(t, sel.Active)
		require.Nil(t, sel.Names)
	})

	t.Run("legacy migrate not setup", func(t *testing.T) {
		t.Parallel()
		sel := setup.Select([]string{"migrate"}, false, "")
		require.False(t, sel.Active)
	})

	t.Run("csv flag", func(t *testing.T) {
		t.Parallel()
		sel := setup.Select(nil, false, "migrate, permissions")
		require.True(t, sel.Active)
		require.Equal(t, []string{"migrate", "permissions"}, sel.Names)
	})

	t.Run("doSetup alone runs all registered", func(t *testing.T) {
		t.Parallel()
		sel := setup.Select(nil, true, "")
		require.True(t, sel.Active)
		require.Nil(t, sel.Names)
	})
}
