package blueprint

type Blueprint struct {
	Service string      `json:"service" yaml:"service"`
	HTTP    []HTTPRoute `json:"http"    yaml:"http"`
	Plugins []Plugin    `json:"plugins" yaml:"plugins"`
	Queues  []Queue     `json:"queues"  yaml:"queues"`
}

type HTTPRoute struct {
	Name     string `json:"name"     yaml:"name"`
	Method   string `json:"method"   yaml:"method"`
	Route    string `json:"route"    yaml:"route"`
	Handler  string `json:"handler"  yaml:"handler"`
	Override bool   `json:"override" yaml:"override"`
	Remove   bool   `json:"remove"   yaml:"remove"`
}

type Plugin struct {
	Name     string            `json:"name"     yaml:"name"`
	Config   map[string]string `json:"config"   yaml:"config"`
	Override bool              `json:"override" yaml:"override"`
	Remove   bool              `json:"remove"   yaml:"remove"`
}

type Queue struct {
	Name       string `json:"name"       yaml:"name"`
	Publisher  string `json:"publisher"  yaml:"publisher"`
	Subscriber string `json:"subscriber" yaml:"subscriber"`
	Topic      string `json:"topic"      yaml:"topic"`
	Override   bool   `json:"override"   yaml:"override"`
	Remove     bool   `json:"remove"     yaml:"remove"`
}
