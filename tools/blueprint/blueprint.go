package blueprint

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Blueprint struct {
	SchemaVersion string        `json:"schema_version" yaml:"schema_version"`
	RuntimeMode   string        `json:"runtime_mode"   yaml:"runtime_mode"`
	Service       *ServiceSpec  `json:"service"        yaml:"service"`
	Services      []ServiceSpec `json:"services"       yaml:"services"`

	HTTP      []HTTPRoute    `json:"http"      yaml:"http"`
	Plugins   []string       `json:"plugins"   yaml:"plugins"`
	Datastore *DatastoreSpec `json:"datastore" yaml:"datastore"`
	Queues    []QueueSpec    `json:"queues"    yaml:"queues"`
}

type ServiceSpec struct {
	Name         string         `json:"name"          yaml:"name"`
	Module       string         `json:"module"        yaml:"module"`
	RuntimeMode  string         `json:"runtime_mode"  yaml:"runtime_mode"`
	ServiceID    any            `json:"service_id"    yaml:"service_id"`
	ServiceGroup string         `json:"service_group" yaml:"service_group"`
	Port         string         `json:"port"          yaml:"port"`
	HTTP         []HTTPRoute    `json:"http"          yaml:"http"`
	Plugins      []string       `json:"plugins"       yaml:"plugins"`
	Datastore    *DatastoreSpec `json:"datastore"     yaml:"datastore"`
	Queues       []QueueSpec    `json:"queues"        yaml:"queues"`
}

type HTTPRoute struct {
	Name     string `json:"name"     yaml:"name"`
	Route    string `json:"route"    yaml:"route"`
	Method   string `json:"method"   yaml:"method"`
	Handler  string `json:"handler"  yaml:"handler"`
	Response string `json:"response" yaml:"response"`
}

type DatastoreSpec struct {
	Migrate       bool   `json:"migrate"         yaml:"migrate"`
	PrimaryURLEnv string `json:"primary_url_env" yaml:"primary_url_env"`
}

type QueueSpec struct {
	Publisher  string `json:"publisher"  yaml:"publisher"`
	Subscriber string `json:"subscriber" yaml:"subscriber"`
	URL        string `json:"url"        yaml:"url"`
	Handler    string `json:"handler"    yaml:"handler"`
}

func (bp *Blueprint) runtimeMode() string {
	if bp == nil {
		return ""
	}
	if strings.TrimSpace(bp.RuntimeMode) != "" {
		return strings.ToLower(strings.TrimSpace(bp.RuntimeMode))
	}
	if bp.Service != nil && strings.TrimSpace(bp.Service.RuntimeMode) != "" {
		return strings.ToLower(strings.TrimSpace(bp.Service.RuntimeMode))
	}
	return ""
}

func LoadFile(path string) (*Blueprint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	bp := &Blueprint{}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		if unmarshalErr := json.Unmarshal(data, bp); unmarshalErr != nil {
			return nil, unmarshalErr
		}
	default:
		if unmarshalErr := yaml.Unmarshal(data, bp); unmarshalErr != nil {
			return nil, unmarshalErr
		}
	}
	return bp, nil
}

func WriteFile(path string, bp *Blueprint) error {
	if bp == nil {
		return errors.New("blueprint is nil")
	}

	var (
		data []byte
		err  error
	)

	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		data, err = json.MarshalIndent(bp, "", "  ")
	default:
		data, err = yaml.Marshal(bp)
	}
	if err != nil {
		return err
	}

	// #nosec G306 -- generated files should be readable by the developer.
	return os.WriteFile(path, data, 0o644)
}
