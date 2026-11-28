package security

import (
	"crypto/sha256"
	"encoding/hex"
)

const tokenFingerprintLength = 8

// TokenFingerprint returns a short, non-reversible identifier for a
// credential (the first 8 hex characters of its SHA-256 digest). Log the
// fingerprint, never the credential, when a token must be correlated across
// log lines. Returns "" for an empty token.
func TokenFingerprint(token string) string {
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])[:tokenFingerprintLength]
}
