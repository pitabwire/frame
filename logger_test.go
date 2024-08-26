package frame_test

import (
	"errors"
	"github.com/pitabwire/frame"
	"runtime/debug"
	"testing"
)

func TestLogs(t *testing.T) {
	ctx, srv := frame.NewService("Logger Srv", frame.Config(
		&frame.ConfigurationDefault{LogLevel: "Debug", Oauth2WellKnownJwk: sampleWellKnownJwk}))

	logger := srv.L(ctx)
	logger.Debug("testing debug logs")
	logger.Info("testing logs")

	err := errors.New("")
	logger.WithError(err).Errorf("testing errors")

	logger.WithError(err).WithField("stacktrace", string(debug.Stack())).Errorf("testing errors with stacktrace")
}
