package frame

import (
	"bytes"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestGetLocalIP(t *testing.T) {

	localIp := GetLocalIP()

	if localIp == "" {
		t.Error("Could not get a local ip even localhost")
	}else if ! strings.Contains(localIp,".") && !strings.Contains(localIp,":"){
		t.Errorf(" The obtained ip %v is not valid ", localIp)
	}

}

func TestGetMacAddress(t *testing.T) {

	macAddress := GetMacAddress()

	if macAddress == "" {
		t.Error("Could not get a mac address for this machine")
	}
}

func TestGetEnv(t *testing.T) {

	env := GetEnv("RANDOM_MISSING_TEST_VALUE", "fallback")

	if env != "fallback" {
		t.Errorf("The environment variable value is expected to be blank but found %v", env)
	}

	err := os.Setenv("RANDOM_EXISTING_TEST_VALUE", "1")
	if err != nil {
		t.Error(err)
	}

	env = GetEnv("RANDOM_EXISTING_TEST_VALUE", "")

	if env != "1" {
		t.Errorf("The environment variable value is expected to be 1 but found %v", env)
	}

}

func TestGetIp(t *testing.T) {

	req, _ := http.NewRequest(http.MethodGet, "", bytes.NewReader([]byte("")))


	ip := GetIp(req)

	if ip != "" {
		t.Errorf("Somehow we found the ip %v yet not expected", ip)
	}

	req.RemoteAddr = "testamentor:80"
	ip = GetIp(req)
	if ip != "testamentor" {
		t.Errorf("Somehow we found the ip %v yet testamentor is expected", ip)
	}


	req.Header.Add("X-FORWARDED-FOR", "testamento")
	ip = GetIp(req)
	if ip != "testamento" {
		t.Errorf("Somehow we found the ip %v yet testamento is expected", ip)
	}

}
