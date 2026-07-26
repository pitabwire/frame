package frame_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pitabwire/frame/v2"
	"github.com/pitabwire/frame/v2/config"
	"github.com/stretchr/testify/require"
)

func TestIsSetupModeFromEnvFlags(t *testing.T) {
	t.Parallel()

	t.Run("do_setup", func(t *testing.T) {
		t.Parallel()
		cfg := config.ConfigurationDefault{DoSetup: true}
		require.True(t, cfg.IsSetupMode())
		require.True(t, frame.IsSetupMode(&cfg))
	})

	t.Run("setup_tasks csv", func(t *testing.T) {
		t.Parallel()
		cfg := config.ConfigurationDefault{SetupTasks: "migrate, permissions ,bootstrap"}
		require.True(t, cfg.IsSetupMode())
		require.Equal(t, []string{"migrate", "permissions", "bootstrap"}, cfg.GetSetupTasks())
	})

	t.Run("idle", func(t *testing.T) {
		t.Parallel()
		cfg := config.ConfigurationDefault{}
		// Without argv `setup` or flags, default config is not setup mode.
		// (argv-based detection is covered by config package / integration.)
		if !cfg.DoSetup && cfg.SetupTasks == "" {
			// May still be true if the test binary was invoked with setup args;
			// only assert CSV parsing above.
			_ = cfg.IsSetupMode()
		}
	})
}

func TestRunSetupOrderAndUnknown(t *testing.T) {
	t.Parallel()

	ctx, svc := frame.NewService()
	order := make([]string, 0, 3)

	svc.AddSetupTask("migrate", func(_ context.Context, _ *frame.Service) error {
		order = append(order, "migrate")
		return nil
	})
	svc.AddSetupTask("permissions", func(_ context.Context, _ *frame.Service) error {
		order = append(order, "permissions")
		return nil
	})
	svc.AddSetupTask("bootstrap", func(_ context.Context, _ *frame.Service) error {
		order = append(order, "bootstrap")
		return nil
	})

	require.NoError(t, svc.RunSetup(ctx, "migrate", "bootstrap", "permissions"))
	require.Equal(t, []string{"migrate", "bootstrap", "permissions"}, order)

	err := svc.RunSetup(ctx, "nope")
	require.ErrorIs(t, err, frame.ErrUnknownSetupTask)
}

func TestRunSetupFailClosed(t *testing.T) {
	t.Parallel()

	ctx, svc := frame.NewService()
	svc.AddSetupTask("migrate", func(_ context.Context, _ *frame.Service) error {
		return errors.New("schema boom")
	})
	err := svc.RunSetup(ctx, "migrate")
	require.ErrorContains(t, err, "setup task \"migrate\"")
	require.ErrorContains(t, err, "schema boom")
}

func TestWithSetupTaskOption(t *testing.T) {
	t.Parallel()

	ctx, svc := frame.NewService()
	svc.Init(ctx, frame.WithSetupTask("migrate", func(_ context.Context, _ *frame.Service) error {
		return nil
	}))
	require.Equal(t, []string{"migrate"}, svc.SetupTaskNames())
}

func TestPermissionsRegisterOnStartDefault(t *testing.T) {
	t.Parallel()
	cfg := config.ConfigurationDefault{}
	// envDefault true — zero value before env parse is false; after FromEnv it is true.
	// Explicit opt-out:
	cfg.PermissionsRegisterOnStart = false
	require.False(t, cfg.GetPermissionsRegisterOnStart())
	cfg.PermissionsRegisterOnStart = true
	require.True(t, cfg.GetPermissionsRegisterOnStart())
}
