package frame_test

import (
	"errors"
	"github.com/pitabwire/frame"
	"runtime/debug"
	"testing"
)

func TestLogs(t *testing.T) {
	ctx, srv := frame.NewService("iLogger Srv", frame.Config(
		&frame.ConfigurationDefault{LogLevel: "Debug", Oauth2WellKnownJwkData: sampleWellKnownJwk}))

	logger := srv.Log(ctx)
	logger.Debug("testing debug logs")
	logger.Info("testing logs")

	err := errors.New("")
	logger.WithError(err).Error("testing errors")

	logger.WithError(err).WithField("stacktrace", string(debug.Stack())).Error("testing errors with stacktrace")
}
