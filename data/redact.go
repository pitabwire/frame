package data

import "github.com/pitabwire/frame/v3/internal/redact"

// RedactedPlaceholder replaces credential material in redacted output.
const RedactedPlaceholder = redact.Placeholder

// RedactURL returns raw with credentials removed so it is safe to log.
// It is shorthand for DSN(raw).Redacted().
func RedactURL(raw string) string {
	return redact.URL(raw)
}

// Redacted returns the DSN with credentials removed so it is safe to log or
// embed in errors: the userinfo password (or a token-only userinfo) and
// sensitive query parameters are replaced with RedactedPlaceholder, as is the
// password of libpq keyword/value DSNs. Strings that net/url cannot parse
// still have their authority userinfo masked.
func (d DSN) Redacted() string {
	return redact.URL(string(d))
}

// ScrubError returns err with every credential of this DSN removed from its
// message. Drivers and net/url routinely echo the full connection URL in
// errors, so errors derived from a DSN must be scrubbed before they are
// logged or surfaced. The original error stays reachable through Unwrap for
// errors.Is / errors.As. Errors that contain no credential are returned as-is.
func (d DSN) ScrubError(err error) error {
	return redact.Error(string(d), err)
}

// ScrubText removes every credential of this DSN from text.
func (d DSN) ScrubText(text string) string {
	return redact.Text(string(d), text)
}
