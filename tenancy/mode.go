package tenancy

import "strings"

// SecurityMode controls how missing, empty, or skipped tenancy claims are
// handled at storage acquire time.
//
// Stock default is ModeFailOpen (historical behaviour). Production services
// should enable ModeHybrid via the Secure Profile.
// See docs/superpowers/specs/2026-07-25-transparent-multi-tenant-isolation.md.
type SecurityMode int

const (
	// ModeFailOpen preserves historical behaviour: nil/empty/Skip → no
	// filter, no error. Default for compatibility.
	ModeFailOpen SecurityMode = iota

	// ModeFailClosed rejects connection acquire when claims are
	// missing/empty/invalid/partition-only, unless an authorized
	// SystemPrincipal authorizes match-all (AllowGlobal) or a scoped
	// SystemPrincipal / normal Claims bind applies.
	ModeFailClosed

	// ModeHybrid is the recommended production posture (Secure Profile).
	// Acquire rules match ModeFailClosed for match-all/Skip in v1.
	ModeHybrid

	modeTokenFailOpen   = "fail_open"
	modeTokenFailClosed = "fail_closed"
	modeTokenHybrid     = "hybrid"
)

// String returns a stable config/env token for the mode.
func (m SecurityMode) String() string {
	switch m {
	case ModeFailOpen:
		return modeTokenFailOpen
	case ModeFailClosed:
		return modeTokenFailClosed
	case ModeHybrid:
		return modeTokenHybrid
	default:
		return modeTokenFailOpen
	}
}

// ParseSecurityMode parses FRAME_TENANCY_SECURITY_MODE style values.
// Empty or unknown values yield ModeFailOpen and false.
func ParseSecurityMode(s string) (SecurityMode, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", modeTokenFailOpen, "fail-open", "open":
		return ModeFailOpen, s != ""
	case modeTokenFailClosed, "fail-closed", "closed":
		return ModeFailClosed, true
	case modeTokenHybrid:
		return ModeHybrid, true
	default:
		return ModeFailOpen, false
	}
}

// IsSecure reports whether the mode rejects unbound acquires.
func (m SecurityMode) IsSecure() bool {
	return m == ModeFailClosed || m == ModeHybrid
}

// ModeAware is implemented by providers that honour SecurityMode.
// Custom WithTenancyProvider values that do not implement ModeAware
// ignore service mode configuration.
type ModeAware interface {
	SetSecurityMode(SecurityMode)
	SecurityMode() SecurityMode
}

// AllowGlobalAware holds the service-name allowlist for AllowGlobal
// SystemPrincipal elevation. Implemented by tenancy/postgres.Provider.
type AllowGlobalAware interface {
	SetAllowGlobalServices(names ...string)
	AllowGlobalServices() []string
}
