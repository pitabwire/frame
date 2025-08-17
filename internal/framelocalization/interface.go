package framelocalization

import "context"

// Configuration for localization behavior
// Keep minimal for now; can be extended to support files, bundles, etc.
type Config interface {
	// DefaultLanguage returns the fallback language tag, e.g. "en"
	DefaultLanguage() string
}

// Manager defines a minimal localization surface to avoid sprinkling
// direct i18n dependencies across modules.
type Manager interface {
	// T returns a localized message for key. If not found, returns key.
	// Optionally accepts formatting args.
	T(ctx context.Context, key string, args ...any) string
	// Close releases any resources if needed.
	Close(ctx context.Context) error
}
