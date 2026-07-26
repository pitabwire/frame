package frame_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pitabwire/frame/v2"
	"github.com/pitabwire/frame/v2/config"
	"github.com/pitabwire/frame/v2/setup"
	"github.com/stretchr/testify/require"
)

func TestServiceSetupRegistryBulk(t *testing.T) {
	t.Parallel()

	ctx, svc := frame.NewService()
	order := make([]string, 0, 3)

	svc.Setup().RegisterFunc(setup.NameMigrate, func(context.Context) error {
		order = append(order, setup.NameMigrate)
		return nil
	})
	svc.AddSetupTask(setup.NamePermissions, func(_ context.Context, _ *frame.Service) error {
		order = append(order, setup.NamePermissions)
		return nil
	})
	svc.Init(ctx, frame.WithSetupFunc(setup.NameBootstrap, func(context.Context) error {
		order = append(order, setup.NameBootstrap)
		return nil
	}))

	require.NoError(t, svc.RunSetup(ctx, setup.NameMigrate, setup.NameBootstrap, setup.NamePermissions))
	require.Equal(t, []string{setup.NameMigrate, setup.NameBootstrap, setup.NamePermissions}, order)
}

func TestServiceRunSetupFailClosed(t *testing.T) {
	t.Parallel()

	ctx, svc := frame.NewService()
	svc.AddSetupTask(setup.NameMigrate, func(_ context.Context, _ *frame.Service) error {
		return errors.New("schema boom")
	})
	err := svc.RunSetup(ctx, setup.NameMigrate)
	require.ErrorContains(t, err, "setup step \"migrate\"")
	require.ErrorContains(t, err, "schema boom")
}

func TestSetupSelectionFromConfig(t *testing.T) {
	t.Parallel()

	cfg := config.ConfigurationDefault{DoSetup: true, SetupTasks: "migrate,permissions"}
	sel := frame.SetupSelection(&cfg)
	require.True(t, sel.Active)
	require.Equal(t, []string{"migrate", "permissions"}, sel.Names)
	require.True(t, frame.IsSetupMode(&cfg))
}

func TestWithSetupStepOption(t *testing.T) {
	t.Parallel()

	ctx, svc := frame.NewService()
	svc.Init(ctx, frame.WithSetupStep(setup.Func{
		StepName: setup.NameVerify,
		Fn:       func(context.Context) error { return nil },
	}))
	require.Equal(t, []string{setup.NameVerify}, svc.SetupTaskNames())
}

func TestPermissionsRegisterOnStartDisabled(t *testing.T) {
	t.Parallel()
	cfg := config.ConfigurationDefault{PermissionsRegisterOnStart: true}
	// Field is ignored — runtime PreStart publishing was removed.
	require.False(t, cfg.GetPermissionsRegisterOnStart())
}

func TestShouldRunSetupAndLegacyMigrate(t *testing.T) {
	t.Parallel()
	ctx, svc := frame.NewService()
	ran := false
	svc.Setup().RegisterFunc(setup.NameMigrate, func(context.Context) error {
		ran = true
		return nil
	})
	cfg := config.ConfigurationDefault{DatabaseMigrate: true}
	require.True(t, frame.ShouldRunSetup(&cfg))
	require.NoError(t, svc.RunSetupForProcess(ctx, &cfg))
	require.True(t, ran)
}
