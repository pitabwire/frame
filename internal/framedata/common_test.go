package framedata_test

import (
	"slices"
	"testing"

	"github.com/pitabwire/frame"
)

type name struct {
	frame.ConfigurationDefault
}

func Test_Config_Process(t *testing.T) {
	t.Setenv("PORT", "testingp")
	t.Setenv("DATABASE_URL", "testingu")
	conf, err := frame.ConfigFromEnv[name]()
	if err != nil {
		t.Errorf(" could not loadOIDC config from env : %v", err)
	}
	if conf.ServerPort != "testingp" {
		t.Errorf("inherited PORT config not processed")
	}
	if !slices.Contains(conf.GetDatabasePrimaryHostURL(), "testingu") {
		t.Errorf("inherited Database URL config not processed")
	}
}

func Test_ConfigCastingIssues(t *testing.T) {
	t.Setenv("PORT", "testingp")
	t.Setenv("DATABASE_URL", "testingu")
	conf, err := frame.ConfigFromEnv[name]()
	if err != nil {
		t.Errorf(" could not loadOIDC config from env : %v", err)
		return
	}
	_, srv := frame.NewService("Test Srv", frame.WithConfig(&conf))
	_, ok := srv.Config().(frame.ConfigurationOAUTH2)
	if !ok {
		t.Errorf("could not cast config to default settings")
	}
}
