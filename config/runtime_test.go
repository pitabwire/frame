package config

import "testing"

func TestNormalizeRuntimeMode(t *testing.T) {
	if got := NormalizeRuntimeMode(""); got != RuntimeModePolylith {
		t.Fatalf("expected polylith, got %s", got)
	}
	if got := NormalizeRuntimeMode("MONOLITH"); got != RuntimeModeMonolith {
		t.Fatalf("expected monolith, got %s", got)
	}
}
