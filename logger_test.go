package frame_test

import (
	"github.com/pitabwire/frame"
	"testing"
)

func TestLogs(t *testing.T) {
	_, srv := frame.NewService("Logger Srv", frame.Config(
		&frame.ConfigurationDefault{Oauth2WellKnownJwk: sampleWellKnownJwk}))

	logger := srv.L()
	logger.Info("testing logs")
}
