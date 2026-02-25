package config_test

import (
	"testing"

	"github.com/pitabwire/frame/config"
)

func TestNormalizeRuntimeMode(t *testing.T) {
	if got := config.NormalizeRuntimeMode(""); got != config.RuntimeModePolylith {
		t.Fatalf("expected polylith, got %s", got)
	}
	if got := config.NormalizeRuntimeMode("MONOLITH"); got != config.RuntimeModeMonolith {
		t.Fatalf("expected monolith, got %s", got)
	}
}
