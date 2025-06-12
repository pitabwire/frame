package frame_test

import (
	"bytes"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/pitabwire/frame"
)

func TestGetLocalIP(t *testing.T) {
	localIP := frame.GetLocalIP()
	if localIP == "" {
		t.Error("Could not get a local ip even localhost")
	} else if !strings.Contains(localIP, ".") && !strings.Contains(localIP, ":") {
		t.Errorf(" The obtained ip %v is not valid ", localIP)
	}
}

func TestGetMacAddress(t *testing.T) {
	macAddress := frame.GetMacAddress()
	if macAddress == "" {
		t.Error("Could not get a mac address for this machine")
	}
}

func TestGetEnv(t *testing.T) {
	env := frame.GetEnv("RANDOM_MISSING_TEST_VALUE", "fallback")
	if env != "fallback" {
		t.Errorf("The environment variable value is expected to be blank but found %v", env)
	}
	t.Setenv("RANDOM_EXISTING_TEST_VALUE", "1")
	env = frame.GetEnv("RANDOM_EXISTING_TEST_VALUE", "")
	if env != "1" {
		t.Errorf("The environment variable value is expected to be 1 but found %v", env)
	}
}

func TestGetIp(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "", bytes.NewReader([]byte("")))
	ip := frame.GetIP(req)
	if ip != "" {
		t.Errorf("Somehow we found the ip %v yet not expected", ip)
	}
	req.RemoteAddr = "testamentor:80"
	ip = frame.GetIP(req)
	if ip != "testamentor" {
		t.Errorf("Somehow we found the ip %v yet testamentor is expected", ip)
	}
	req.Header.Add("X-Forwarded-For", "testamento")
	ip = frame.GetIP(req)
	if ip != "testamento" {
		t.Errorf("Somehow we found the ip %v yet testamento is expected", ip)
	}
}

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
