package framelocalization

import "context"

// NoopConfig provides a simple Config implementation
// with a static default language.
type NoopConfig struct{ Lang string }

func (c NoopConfig) DefaultLanguage() string { if c.Lang == "" { return "en" }; return c.Lang }

// NoopManager implements Manager by returning keys as-is.
// Useful as a default when i18n catalogs are not wired yet.
type NoopManager struct{ cfg Config }

func NewNoopManager(cfg Config) *NoopManager { return &NoopManager{cfg: cfg} }

func (m *NoopManager) T(_ context.Context, key string, _ ...any) string { return key }

func (m *NoopManager) Close(_ context.Context) error { return nil }
