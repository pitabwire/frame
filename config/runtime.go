package config

import "strings"

const (
	RuntimeModeMonolith = "monolith"
	RuntimeModePolylith = "polylith"
)

func NormalizeRuntimeMode(mode string) string {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		return RuntimeModePolylith
	}
	if mode != RuntimeModeMonolith {
		return RuntimeModePolylith
	}
	return mode
}

func IsMonolith(cfg ConfigurationRuntime) bool {
	if cfg == nil {
		return false
	}
	return NormalizeRuntimeMode(cfg.RuntimeMode()) == RuntimeModeMonolith
}

func IsPolylith(cfg ConfigurationRuntime) bool {
	if cfg == nil {
		return true
	}
	return NormalizeRuntimeMode(cfg.RuntimeMode()) == RuntimeModePolylith
}
